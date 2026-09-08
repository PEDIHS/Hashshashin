package main

import (
 "context"
 "crypto/aes"
 "crypto/cipher"
 "crypto/hmac"
 "crypto/rand"
 "crypto/sha256"
 "encoding/base64"
 "encoding/binary"
 "encoding/json"
 "errors"
 "flag"
 "fmt"
 "io"
 "log"
 "net"
 "os"
 "os/exec"
 "os/signal"
 "strings"
 "sync/atomic"
 "syscall"
 "time"
)

type Config struct {
 Role string `json:"role"`; Mode string `json:"mode"`
 Transport struct { Listen, Peer, Key string } `json:"transport"`
 Tun struct { Name, LocalCIDR, PeerIP string; MTU int } `json:"tun"`
 Network struct { PublicInterface, PublicIP, ForeignPublicIP, IranPublicIP string } `json:"network"`
 Ports []struct { Port int; Protocol string } `json:"ports"`
}
const ( magic="HSH1"; msgHello=1; msgHelloAck=2; msgData=3 )
type session struct { txAEAD,rxAEAD cipher.AEAD; tx,rx uint64; peer *net.UDPAddr }

func main(){ p:=flag.String("c","/etc/hashshashin/config.json","config"); check:=flag.Bool("check",false,"validate only"); flag.Parse(); c,e:=loadConfig(*p); if e!=nil{log.Fatal(e)}; if *check{fmt.Println("config ok");return}; if os.Geteuid()!=0{log.Fatal("must run as root")}; if e=run(c);e!=nil{log.Fatal(e)} }
func loadConfig(path string)(*Config,error){ b,e:=os.ReadFile(path);if e!=nil{return nil,e};var c Config;if e=json.Unmarshal(b,&c);e!=nil{return nil,e};if c.Role!="iran"&&c.Role!="kharej"{return nil,errors.New("role must be iran or kharej")};if c.Mode!="full"&&c.Mode!="direct-return"{return nil,errors.New("mode must be full or direct-return")};if c.Tun.Name==""{c.Tun.Name="hsh0"};if c.Tun.MTU==0{c.Tun.MTU=1300};kb,e:=base64.StdEncoding.DecodeString(c.Transport.Key);if e!=nil||len(kb)!=32{return nil,errors.New("transport.key must be base64 of exactly 32 bytes")};if c.Transport.Listen==""{return nil,errors.New("transport.listen required")};if c.Role=="iran"&&c.Transport.Peer==""{return nil,errors.New("transport.peer required on iran")};if c.Tun.LocalCIDR==""{return nil,errors.New("tun.local_cidr required")};return &c,nil }

func run(c *Config)error{ tun,e:=openTun(c.Tun.Name);if e!=nil{return e};defer tun.Close();if e=setupTun(c);e!=nil{return e};if e=setupRouting(c);e!=nil{return e};a,e:=net.ResolveUDPAddr("udp",c.Transport.Listen);if e!=nil{return e};u,e:=net.ListenUDP("udp",a);if e!=nil{return e};defer u.Close();key,_:=base64.StdEncoding.DecodeString(c.Transport.Key);ctx,cancel:=signal.NotifyContext(context.Background(),syscall.SIGINT,syscall.SIGTERM);defer cancel();var s atomic.Pointer[session];go recvLoop(u,tun,key,&s,c.Role);if c.Role=="iran"{peer,e:=net.ResolveUDPAddr("udp",c.Transport.Peer);if e!=nil{return e};if e=handshakeClient(ctx,u,peer,key,&s);e!=nil{return e}};go func(){<-ctx.Done();u.Close();tun.Close()}();buf:=make([]byte,65535);for{n,e:=tun.Read(buf);if e!=nil{if ctx.Err()!=nil{return nil};return e};ss:=s.Load();if ss!=nil{if e=sendData(u,ss,buf[:n]);e!=nil{log.Printf("send: %v",e)}}} }

func handshakeClient(ctx context.Context,u *net.UDPConn,peer *net.UDPAddr,key []byte,dst *atomic.Pointer[session])error{cn:=make([]byte,32);rand.Read(cn);p:=append([]byte(magic),msgHello);p=append(p,cn...);m:=hmac.New(sha256.New,key);m.Write(p);p=append(p,m.Sum(nil)...);deadline:=time.Now().Add(12*time.Second);for{if _,e:=u.WriteToUDP(p,peer);e!=nil{return e};for i:=0;i<10;i++{if dst.Load()!=nil{return nil};if time.Now().After(deadline){return errors.New("handshake timeout")};select{case<-ctx.Done():return ctx.Err();case<-time.After(100*time.Millisecond):}}} }
func recvLoop(u *net.UDPConn,tun io.Writer,key []byte,dst *atomic.Pointer[session],role string){b:=make([]byte,65535);for{n,peer,e:=u.ReadFromUDP(b);if e!=nil{return};if n<5||string(b[:4])!=magic{continue};switch b[4]{case msgHello:if role!="kharej"||n!=69{continue};m:=hmac.New(sha256.New,key);m.Write(b[:37]);if !hmac.Equal(m.Sum(nil),b[37:69]){continue};cn:=append([]byte(nil),b[5:37]...);sn:=make([]byte,32);rand.Read(sn);ss,e:=deriveSession(key,cn,sn,peer,false);if e!=nil{continue};dst.Store(ss);o:=append([]byte(magic),msgHelloAck);o=append(o,cn...);o=append(o,sn...);hm:=hmac.New(sha256.New,key);hm.Write(o);o=append(o,hm.Sum(nil)...);u.WriteToUDP(o,peer);case msgHelloAck:if role!="iran"||n!=101{continue};hm:=hmac.New(sha256.New,key);hm.Write(b[:69]);if !hmac.Equal(hm.Sum(nil),b[69:101]){continue};ss,e:=deriveSession(key,b[5:37],b[37:69],peer,true);if e==nil{dst.Store(ss)};case msgData:ss:=dst.Load();if ss==nil||!peer.IP.Equal(ss.peer.IP)||peer.Port!=ss.peer.Port||n<41{continue};ctr:=binary.BigEndian.Uint64(b[5:13]);if ctr<=atomic.LoadUint64(&ss.rx){continue};nonce:=b[13:25];plain,e:=ss.rxAEAD.Open(nil,nonce,b[25:n],b[:13]);if e!=nil{continue};for{old:=atomic.LoadUint64(&ss.rx);if ctr<=old{plain=nil;break};if atomic.CompareAndSwapUint64(&ss.rx,old,ctr){break}};if plain!=nil{tun.Write(plain)}}} }
func deriveSession(psk,cn,sn []byte,peer *net.UDPAddr,initiator bool)(*session,error){base:=func(label string)[]byte{h:=hmac.New(sha256.New,psk);h.Write([]byte("hashshashin-v0/"+label));h.Write(cn);h.Write(sn);return h.Sum(nil)};c2s:=base("c2s");s2c:=base("s2c");mk:=func(k []byte)(cipher.AEAD,error){b,e:=aes.NewCipher(k[:32]);if e!=nil{return nil,e};return cipher.NewGCM(b)};a,e:=mk(c2s);if e!=nil{return nil,e};b,e:=mk(s2c);if e!=nil{return nil,e};if initiator{return &session{txAEAD:a,rxAEAD:b,peer:peer},nil};return &session{txAEAD:b,rxAEAD:a,peer:peer},nil }
func sendData(u *net.UDPConn,s *session,p []byte)error{ctr:=atomic.AddUint64(&s.tx,1);hdr:=make([]byte,13);copy(hdr[:4],magic);hdr[4]=msgData;binary.BigEndian.PutUint64(hdr[5:],ctr);nonce:=make([]byte,12);binary.BigEndian.PutUint64(nonce[4:],ctr);ct:=s.txAEAD.Seal(nil,nonce,p,hdr);o:=append(hdr,nonce...);o=append(o,ct...);_,e:=u.WriteToUDP(o,s.peer);return e}

func setupTun(c *Config)error{if e:=runCmd("ip","link","set","dev",c.Tun.Name,"up","mtu",fmt.Sprint(c.Tun.MTU));e!=nil{return e};return runCmd("ip","addr","replace",c.Tun.LocalCIDR,"dev",c.Tun.Name)}
func setupRouting(c *Config)error{if len(c.Ports)==0{return nil};if c.Network.PublicInterface==""||c.Network.PublicIP==""||c.Network.ForeignPublicIP==""||c.Network.IranPublicIP==""{return errors.New("network public_interface/public_ip/foreign_public_ip/iran_public_ip required when ports are configured")};runCmd("sysctl","-w","net.ipv4.ip_forward=1");runCmd("sysctl","-w","net.ipv4.conf."+c.Tun.Name+".rp_filter=2");if c.Role=="iran"{runCmd("ip","rule","del","fwmark","0x66","table","166");runCmd("ip","route","flush","table","166");if e:=runCmd("ip","rule","add","fwmark","0x66","table","166");e!=nil{return e};if e:=runCmd("ip","route","add",c.Network.ForeignPublicIP+"/32","dev",c.Tun.Name,"table","166");e!=nil{return e};for _,p:=range c.Ports{ps:=[]string{p.Protocol};if p.Protocol=="both"{ps=[]string{"tcp","udp"}};for _,pr:=range ps{ensureIPT("mangle","PREROUTING","-i",c.Network.PublicInterface,"-p",pr,"--dport",fmt.Sprint(p.Port),"-j","MARK","--set-mark","0x66");ensureIPT("nat","PREROUTING","-i",c.Network.PublicInterface,"-p",pr,"--dport",fmt.Sprint(p.Port),"-j","DNAT","--to-destination",fmt.Sprintf("%s:%d",c.Network.ForeignPublicIP,p.Port))}};ensureIPT("nat","POSTROUTING","-o",c.Tun.Name,"-d",c.Network.ForeignPublicIP,"-j","SNAT","--to-source",c.Network.IranPublicIP)}else if c.Mode=="full"{runCmd("ip","route","replace",c.Network.IranPublicIP+"/32","dev",c.Tun.Name)};return nil}
func ensureIPT(table,chain string,args ...string){check:=append([]string{"-t",table,"-C",chain},args...);if exec.Command("iptables",check...).Run()==nil{return};add:=append([]string{"-t",table,"-A",chain},args...);if o,e:=exec.Command("iptables",add...).CombinedOutput();e!=nil{log.Printf("iptables: %v %s",e,o)}}
func runCmd(n string,a ...string)error{c:=exec.Command(n,a...);o,e:=c.CombinedOutput();if e!=nil{return fmt.Errorf("%s %s: %v: %s",n,strings.Join(a," "),e,o)};return nil}
