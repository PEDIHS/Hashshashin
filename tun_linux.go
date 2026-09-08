//go:build linux

package main

import (
 "fmt"
 "os"
 "syscall"
 "unsafe"
)

const (
 tunSetIFF = 0x400454ca
 iffTun = 0x0001
 iffNoPI = 0x1000
)

type ifreq struct {
 Name [16]byte
 Flags uint16
 Pad [22]byte
}

func openTun(name string)(*os.File,error){
 f,err:=os.OpenFile("/dev/net/tun",os.O_RDWR,0); if err!=nil{return nil,err}
 var req ifreq; copy(req.Name[:],[]byte(name)); req.Flags=iffTun|iffNoPI
 _,_,errno:=syscall.Syscall(syscall.SYS_IOCTL,f.Fd(),uintptr(tunSetIFF),uintptr(unsafe.Pointer(&req)))
 if errno!=0 {f.Close(); return nil,fmt.Errorf("TUNSETIFF: %v",errno)}
 return f,nil
}
