package bot

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tranducnguyen/util_core/filehandle"
	"github.com/tranducnguyen/util_core/global"
	"github.com/tranducnguyen/util_core/proxy_pool"
)

var (
	BotManagerInstace *BotManager
	once              sync.Once
	defaultMaxRetries int32 = 3
)

func SetDefaultMaxRetries(n int) {
	if n < 0 {
		n = 0
	}
	atomic.StoreInt32(&defaultMaxRetries, int32(n))
}

func GetInstance() *BotManager {
	once.Do(func() {
		var botNum = 1
		if global.Config.Bot.NumberBot > 0 {
			botNum = global.NumberBot
		}
		BotManagerInstace = NewBotManager(*global.GContext, botNum)
	})
	return BotManagerInstace
}

// BotData chứa thông tin cần xử lý.
type BotData struct {
	Proxy    *proxy_pool.ProxyInfo
	User     string
	PassWord string
	Status   STATUS
	Data     []byte
	Resp     []byte
	Err      string
	Retries  int
}

type BotFunc func(ctx context.Context, data BotData) (BotResp, error)

type ErrFunc func(err error)

type BotLog func(msg string) error

// BotManager quản lý pool workers để xử lý BotData.
type BotManager struct {
	ctx         context.Context
	cancel      context.CancelFunc
	fn          BotFunc
	erFn        ErrFunc
	tasks       chan BotData
	result      chan BotResp
	retries     chan BotData
	wg          sync.WaitGroup
	activeTasks int64
	maxRetries  int32
	summaryMu   sync.Mutex
	summary     map[STATUS]int
	hitFile     *filehandle.WriteHandler
	errFile     *filehandle.WriteHandler
	checkFile   *filehandle.WriteHandler
	log         BotLog
	isStopped   bool
}

func NewBotManager(parentCtx context.Context, poolSize int) *BotManager {
	ctx, cancel := context.WithCancel(parentCtx)
	fileResult, _ := filehandle.NewWriteHandler("hit.txt", 1024)
	fileFailed, _ := filehandle.NewWriteHandler("error.txt", 1024)
	fileCheck, _ := filehandle.NewWriteHandler("check.txt", 1024)
	m := &BotManager{
		ctx:    ctx,
		cancel: cancel,
		// buffer tasks channel = poolSize để Submit sẽ block khi full
		tasks:      make(chan BotData, poolSize),
		retries:    make(chan BotData, poolSize),
		result:     make(chan BotResp, poolSize),
		summary:    make(map[STATUS]int),
		hitFile:    fileResult,
		errFile:    fileFailed,
		checkFile:  fileCheck,
		maxRetries: atomic.LoadInt32(&defaultMaxRetries),
	}
	m.wg.Add(poolSize)
	for i := 0; i < poolSize; i++ {
		go m.worker()
	}
	return m
}

func (m *BotManager) SetFn(fn BotFunc) {
	m.fn = fn
}

func (m *BotManager) SetMaxRetries(n int) {
	if n < 0 {
		n = 0
	}
	atomic.StoreInt32(&m.maxRetries, int32(n))
}

func (m *BotManager) IsStopped() bool {
	m.summaryMu.Lock()
	defer m.summaryMu.Unlock()
	return m.isStopped
}

func (m *BotManager) Stop() {
	m.summaryMu.Lock()
	defer m.summaryMu.Unlock()
	m.isStopped = true
	m.log("BotManager is stopped")
}

func (m *BotManager) Submit(data BotData) {
	if m.IsStopped() {
		m.log("BotManager is stopped, cannot submit new tasks")
		return
	}

	select {
	case <-m.ctx.Done():
		return
	case m.tasks <- data:
	}
}

func (m *BotManager) SetHitFile(filePtr *filehandle.WriteHandler) {
	m.hitFile = filePtr
}

func (m *BotManager) Wait() {
	m.wg.Wait()
}

func (m *BotManager) WaitingToOutOfTasks() {
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	stableCount := 0

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			if m.IsAllTaskDone() && atomic.LoadInt64(&m.activeTasks) == 0 {
				stableCount++
				if stableCount >= 3 {
					m.log("All channels are empty, all tasks completed")
					return
				}
			} else {
				stableCount = 0
			}
		}
	}
}

func (m *BotManager) IsAllTaskDone() bool {
	return len(m.tasks) == 0 &&
		len(m.retries) == 0
}

func (m *BotManager) Shutdown() {
	m.WaitingToOutOfTasks()
	m.Stop()
	m.cancel()
	m.wg.Wait()
	close(m.tasks)
	close(m.retries)

	m.errFile.Close()
	m.checkFile.Close()
	m.hitFile.Close()
}
func (m *BotManager) SetLog(log BotLog) {
	m.log = log
}

func (m *BotManager) SetErrorFn(fn ErrFunc) {
	m.erFn = fn
}

func (m *BotManager) Log(msg string) {
	m.log(msg)
}

func (m *BotManager) AddWait() {
	atomic.AddInt64(&m.activeTasks, 1)
}
func (m *BotManager) DoneWait() {
	atomic.AddInt64(&m.activeTasks, -1)
}

func (m *BotManager) OnError(err error) {
	if m.erFn != nil {
		m.erFn(err)
	} else {
		m.log(fmt.Sprintf("Error: %v", err))
	}
}

func (m *BotManager) handle(data BotData, label string, qlen, qcap int) {
	m.AddWait()
	defer m.DoneWait()

	if label == "" {
		label = "Task"
	}
	m.log(fmt.Sprintf("Do %s [%v,%v]", label, qlen, qcap))
	resp, err := m.fn(m.ctx, data)

	if err != nil {
		m.IncreNum(Error)
		m.OnError(err)
		return
	}

	if resp.Type == Retries {
		maxRetries := int(atomic.LoadInt32(&m.maxRetries))
		if data.Retries >= maxRetries {
			resp.Type = Bad
			m.IncreNum(resp.Type)
			m.SaveData(resp)
			return
		}

		m.IncreNum(resp.Type)
		data.Retries++
		m.enqueueRetry(data)
		return
	}

	m.IncreNum(resp.Type)
	m.SaveData(resp)
}

func (m *BotManager) enqueueRetry(data BotData) {
	m.AddWait()
	go func() {
		defer m.DoneWait()
		select {
		case <-m.ctx.Done():
			return
		case m.retries <- data:
		}
	}()
}

func (m *BotManager) HandleTask(data BotData) {
	m.handle(data, "Task", len(m.tasks), cap(m.tasks))
}

func (m *BotManager) HandleRetry(data BotData) {
	label := "Retry"
	if data.Retries > 0 {
		label = fmt.Sprintf("Retry%d", data.Retries)
	}
	m.handle(data, label, len(m.retries), cap(m.retries))
}

func (m *BotManager) HandleRetry1(data BotData) {
	m.HandleRetry(data)
}

func (m *BotManager) HandleRetry2(data BotData) {
	m.HandleRetry(data)
}

func (m *BotManager) worker() {
	defer m.wg.Done()
	for {
		select {
		case <-m.ctx.Done():
			return
		case data, ok := <-m.tasks:
			if !ok {
				return
			}
			m.HandleTask(data)

		case data, ok := <-m.retries:
			if !ok {
				m.log(fmt.Sprintf("Do Retry %v", ok))
				return
			}
			m.HandleRetry(data)
		}

	}
}

func (m *BotManager) RetriesProcess(botData BotData) error {
	maxRetries := int(atomic.LoadInt32(&m.maxRetries))
	if botData.Retries >= maxRetries {
		return fmt.Errorf("max time retries")
	}
	botData.Retries++
	m.enqueueRetry(botData)
	return nil
}

func (m *BotManager) IncreNum(typeData STATUS) {
	m.summaryMu.Lock()
	m.summary[typeData]++
	m.summaryMu.Unlock()
}

func (m *BotManager) Summary() map[STATUS]int {
	m.summaryMu.Lock()
	defer m.summaryMu.Unlock()
	copy := make(map[STATUS]int, len(m.summary))
	for k, v := range m.summary {
		copy[k] = v
	}
	return copy
}

func (m *BotManager) SaveData(data BotResp) error {
	m.log(fmt.Sprintf("save data : %v", StatusDescription[data.Type]))
	if len(data.Data) == 0 {
		return nil
	}

	switch data.Type {
	case Hit:
		return m.hitFile.WriteLines(data.Data, true)
	case Bad:
		return m.errFile.WriteLines(data.Data, true)
	case ToCheck:
		return m.checkFile.WriteLines(data.Data, true)
	}
	return nil
}
