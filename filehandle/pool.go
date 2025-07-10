package filehandle

import (
	"bufio"
	"sync"
)

var (
	readerPool = sync.Pool{
		New: func() interface{} {
			return bufio.NewReaderSize(nil, 4*1024) // 4MB
		},
	}

	writerPool = sync.Pool{
		New: func() interface{} {
			return bufio.NewWriterSize(nil, 4*1024) // 4MB
		},
	}
)
