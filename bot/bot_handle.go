package bot

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/tranducnguyen/util_core/filehandle"
	"github.com/tranducnguyen/util_core/global"
	"github.com/tranducnguyen/util_core/proxy_pool"
)

var (
	BotManagerInstace *BotManager
	once              sync.Once
)

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

type BotLog func(msg string) error

// BotManager quản lý pool workers để xử lý BotData.
type BotManager struct {
	ctx       context.Context
	cancel    context.CancelFunc
	fn        BotFunc
	tasks     chan BotData
	result    chan BotResp
	retries   chan BotData
	retries1  chan BotData
	retries2  chan BotData
	wg        sync.WaitGroup
	wgTask    sync.WaitGroup
	summaryMu sync.Mutex
	summary   map[STATUS]int
	hitFile   *filehandle.WriteHandler
	errFile   *filehandle.WriteHandler
	checkFile *filehandle.WriteHandler
	log       BotLog
	isStopped bool
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
		tasks:     make(chan BotData, poolSize),
		retries:   make(chan BotData, poolSize),
		retries1:  make(chan BotData, poolSize),
		retries2:  make(chan BotData, poolSize),
		result:    make(chan BotResp, poolSize),
		summary:   make(map[STATUS]int),
		hitFile:   fileResult,
		errFile:   fileFailed,
		checkFile: fileCheck,
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
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			if m.IsAllTaskDone() {
				m.wgTask.Wait()
				time.Sleep(500 * time.Millisecond)
				if m.IsAllTaskDone() {
					m.log("All channels are empty, all tasks completed")
					return
				}
			}
		}
	}
}

func (m *BotManager) IsAllTaskDone() bool {
	return len(m.tasks) == 0 &&
		len(m.retries) == 0 &&
		len(m.retries1) == 0 &&
		len(m.retries2) == 0
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

func (m *BotManager) Log(msg string) {
	m.log(msg)
}

func (m *BotManager) AddWait() {
	m.wgTask.Add(1)
}
func (m *BotManager) DoneWait() {
	m.wgTask.Done()
}

func (m *BotManager) HandleTask(data BotData) {
	m.wgTask.Add(1)
	defer m.wgTask.Done()
	m.log(fmt.Sprintf("Do Task [%v,%v]", len(m.tasks), cap(m.tasks)))
	resp, err := m.fn(m.ctx, data)

	if err != nil {
		m.log(err.Error())
		m.IncreNum(Error)
		return
	}

	m.IncreNum(resp.Type)
	if resp.Type == Retries {
		go func() {
			m.retries <- data
		}()

	} else {
		m.SaveData(resp)
	}
}

func (m *BotManager) HandleRetry(data BotData) {
	m.log(fmt.Sprintf("Do Retry [%v,%v]", len(m.retries), cap(m.retries)))
	m.wgTask.Add(1)
	defer m.wgTask.Done()
	resp, err := m.fn(m.ctx, data)

	if err != nil {
		m.IncreNum(Error)
		return
	}

	m.IncreNum(resp.Type)

	if resp.Type == Retries {
		m.Log("Send to retry1")
		go func() {
			m.retries1 <- data
		}()

	} else {
		m.SaveData(resp)
	}
}

func (m *BotManager) HandleRetry1(data BotData) {
	m.log(fmt.Sprintf("Do Retry1 [%v:%v]", len(m.retries1), cap(m.retries1)))
	m.wgTask.Add(1)
	defer m.wgTask.Done()
	resp, err := m.fn(m.ctx, data)

	if err != nil {
		m.IncreNum(Error)
		return
	}

	m.IncreNum(resp.Type)
	if resp.Type == Retries {
		m.Log("Send to retry")
		go func() {
			m.retries <- data
		}()

	} else {
		m.SaveData(resp)
	}
}

func (m *BotManager) HandleRetry2(data BotData) {
	m.log(fmt.Sprintf("Do Retry2 [%v:%v]", len(m.retries2), cap(m.retries2)))
	m.wgTask.Add(1)
	defer m.wgTask.Done()
	resp, err := m.fn(m.ctx, data)

	if err != nil {
		m.IncreNum(Error)
		return
	}

	if resp.Type == Retries {
		resp.Type = Bad
	}
	m.IncreNum(resp.Type)
	m.SaveData(resp)
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

		case data, ok := <-m.retries1:
			if !ok {
				m.log(fmt.Sprintf("Do Retry1 %v", ok))
				return
			}
			m.HandleRetry1(data)

		case data, ok := <-m.retries2:
			if !ok {
				m.log(fmt.Sprintf("Do Retry2 %v", ok))
				return
			}
			m.HandleRetry2(data)
		}

	}
}

func (m *BotManager) RetriesProcess(botData BotData) error {
	if botData.Retries > 3 {
		return fmt.Errorf("max time retries")
	}
	botData.Retries++

	if condition := botData.Retries % 2; condition == 0 {
		m.retries1 <- botData
	} else {
		m.retries2 <- botData
	}
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
