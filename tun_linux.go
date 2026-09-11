//go:build linux

package main

import (
	"fmt"
	"io"
	"os"
	"sync/atomic"
	"syscall"
	"unsafe"
)

const (
	tunSetIFF     = 0x400454ca
	iffTun        = 0x0001
	iffMultiQueue = 0x0100
	iffNoPI       = 0x1000
)

type ifreq struct {
	Name  [16]byte
	Flags uint16
	Pad   [22]byte
}

type tunDevice struct {
	name   string
	queues []*os.File
	write  uint32
}

func openTun(name string, queueCount int) (*tunDevice, error) {
	if queueCount < 1 {
		queueCount = 1
	}
	dev, err := openTunWithQueues(name, queueCount)
	if err == nil {
		return dev, nil
	}
	if queueCount == 1 {
		return nil, err
	}

	// Some virtualized kernels expose TUN but reject IFF_MULTI_QUEUE. Do not
	// make that a startup failure: close the partial interface and retry with a
	// classic single queue. The runtime logs the actual queue count so this
	// downgrade is visible during performance diagnosis.
	return openTunWithQueues(name, 1)
}

func openTunWithQueues(name string, queueCount int) (*tunDevice, error) {
	files := make([]*os.File, 0, queueCount)
	multi := queueCount > 1
	for i := 0; i < queueCount; i++ {
		f, err := openTunQueue(name, multi)
		if err != nil {
			for _, q := range files {
				_ = q.Close()
			}
			return nil, fmt.Errorf("open TUN queue %d/%d: %w", i+1, queueCount, err)
		}
		files = append(files, f)
	}
	return &tunDevice{name: name, queues: files}, nil
}

func openTunQueue(name string, multi bool) (*os.File, error) {
	f, err := os.OpenFile("/dev/net/tun", os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	var req ifreq
	copy(req.Name[:], []byte(name))
	req.Flags = iffTun | iffNoPI
	if multi {
		req.Flags |= iffMultiQueue
	}
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), uintptr(tunSetIFF), uintptr(unsafe.Pointer(&req)))
	if errno != 0 {
		_ = f.Close()
		return nil, fmt.Errorf("TUNSETIFF: %v", errno)
	}
	return f, nil
}

func (t *tunDevice) QueueCount() int { return len(t.queues) }

func (t *tunDevice) ReadQueue(index int, p []byte) (int, error) {
	if index < 0 || index >= len(t.queues) {
		return 0, fmt.Errorf("invalid TUN queue index %d", index)
	}
	return t.queues[index].Read(p)
}

// Write injects a packet back into the kernel. Receive workers may call Write
// concurrently; spreading those writes across queue FDs avoids turning the
// return path into another single-FD bottleneck.
func (t *tunDevice) Write(p []byte) (int, error) {
	if len(t.queues) == 0 {
		return 0, io.ErrClosedPipe
	}
	n := atomic.AddUint32(&t.write, 1)
	q := t.queues[int(n-1)%len(t.queues)]
	return q.Write(p)
}

func (t *tunDevice) Close() error {
	var first error
	for _, q := range t.queues {
		if err := q.Close(); err != nil && first == nil {
			first = err
		}
	}
	t.queues = nil
	return first
}
