package main

import (
 "bytes"
 "crypto/rand"
 "net"
 "testing"
)

func TestDirectionalSessionKeys(t *testing.T){
 psk:=make([]byte,32);cn:=make([]byte,32);sn:=make([]byte,32);rand.Read(psk);rand.Read(cn);rand.Read(sn);peer:=&net.UDPAddr{IP:net.IPv4(127,0,0,1),Port:9000}
 client,err:=deriveSession(psk,cn,sn,peer,true);if err!=nil{t.Fatal(err)}
 server,err:=deriveSession(psk,cn,sn,peer,false);if err!=nil{t.Fatal(err)}
 aad:=[]byte("header");nonce:=make([]byte,12);nonce[11]=1;msg:=[]byte("hashshashin")
 ct:=client.txAEAD.Seal(nil,nonce,msg,aad);got,err:=server.rxAEAD.Open(nil,nonce,ct,aad);if err!=nil||!bytes.Equal(got,msg){t.Fatalf("c2s decrypt failed: %v",err)}
 ct=server.txAEAD.Seal(nil,nonce,msg,aad);got,err=client.rxAEAD.Open(nil,nonce,ct,aad);if err!=nil||!bytes.Equal(got,msg){t.Fatalf("s2c decrypt failed: %v",err)}
 if bytes.Equal(client.txAEAD.Seal(nil,nonce,msg,aad),server.txAEAD.Seal(nil,nonce,msg,aad)){t.Fatal("directional keys must differ")}
}
