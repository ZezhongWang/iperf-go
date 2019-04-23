package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"reflect"
	"time"
)

const SIZE = 128*1024

func handleConn(c net.Conn) {
	totalBytes := 0
	buffer := make([]byte, SIZE)
	//ctrl_chan := make(chan uint, 5)
	for i := 0; i< 10; i++{
		n, err := c.Read(buffer)
		totalBytes += n
		fmt.Printf("server read totalBytes = %v n = %v\n", totalBytes, n)
		if err != nil {
			if err != io.EOF {
				fmt.Printf("Read error: %s", err)
			}
			break
		}
		n, err = c.Write(buffer)
		fmt.Printf("server echo n = %v\n", n)
	}
	fmt.Printf("Server stop\n")
	c.Close()
	//for {
	//	time.Sleep(1)
	//	fmt.Printf("do something\n")
	//}
}


func server(){
	Address := "127.0.0.1:9999"
	Addr, err := net.ResolveTCPAddr("tcp", Address)
	if err != nil {
		fmt.Printf("err = %v",err)
	}

	listener, err := net.ListenTCP("tcp", Addr)
	if err != nil {
		fmt.Printf("err = %v",err)
	}
	defer listener.Close()

	//server loop
	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}
		go handleConn(conn)
		return
	}
}

func client(){
	tcpAddr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:9999")
	if err != nil {
		fmt.Printf("err = %v",err)
	}
	conn, err := net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		fmt.Printf("err = %v",err)
	}
	defer conn.Close()
	buffer := make([]byte, SIZE)
	total_size := 0
	//if err := conn.SetWriteBuffer(SIZE); err != nil{
	//	fmt.Printf("err = %v",err)
	//}

	for i := 0 ; i < 10 ; i++ {
		n, err := conn.Write(buffer)
		total_size += n
		fmt.Printf("client write total_size = %v n = %v\n", total_size, n)
		start := time.Now()
		if serr, ok := err.(*net.OpError); ok{
			fmt.Printf("write serr = %T %+v %v\n", serr, serr, reflect.TypeOf(serr))
			fmt.Printf("aaaa = %T %+v %v\n", serr.Err, serr.Err, reflect.TypeOf(serr.Err))
			fmt.Printf("aaaa = %T %+v %v\n", serr.Err.(*os.SyscallError).Err, serr.Err.(*os.SyscallError).Err, reflect.TypeOf(serr.Err.(*os.SyscallError).Err))
		}

		n, err = conn.Read(buffer)
		fmt.Printf("client recv echo n = %v\n", n)
		end := time.Now()
		if err != nil {
			fmt.Printf("write err = %T %+v %v\n", err, err, reflect.TypeOf(err))
		}
		fmt.Printf("RTT = %v start = %v end = %v\n", end.Sub(start).Nanoseconds(), start.Nanosecond(), end.Nanosecond())

		//info := getTCP_Info(conn)
		//fmt.Printf("Rtt:%v\tRecv_RTT:%v\tSend_ss:%v\tRcv_ss:%v\tSnd_mss:%v\nAck_recv:%v\nAck_send:%v\ndata_recv:%v\ndata_sent:%v\n",
		//	info.Rtt, info.Rcv_rtt, info.Snd_ssthresh, info.Rcv_ssthresh, info.Snd_mss, info.Last_ack_recv, info.Last_ack_sent, info.Last_data_recv, info.Last_data_sent)
	}
	fmt.Printf("Client stop\n")
}
//
//func getTCP_Info(conn net.Conn) *unix.TCPInfo{
//	file, err:= conn.(*net.TCPConn).File()
//	if err != nil {
//		fmt.Printf("File err :%v", err)
//	}
//	fd := file.Fd()
//	info, err := unix.GetsockoptTCPInfo(int(fd), unix.SOL_TCP, unix.TCP_INFO)
//	return info
//}
//
//func main(){
//	go server()
//	go client()
//	time.Sleep(time.Second *10)
//}