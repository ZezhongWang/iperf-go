package main

import (
	"fmt"
	"golang.org/x/sys/unix"
	"net"
)

func save_tcpInfo(sp *iperf_stream, rp *iperf_interval_results) int{
	if has_tcpInfo() != true{
		return -1
	}
	info := getTCPInfo(sp.conn)
	rp.rtt = uint(info.Rtt)
	rp.interval_retrans = uint(info.Retrans) // temporarily store
	//sp.test.r info.Retrans
	return 0
}

func getTCPInfo(conn net.Conn) *unix.TCPInfo{
	file, err:= conn.(*net.TCPConn).File()
	if err != nil {
		fmt.Printf("File err :%v", err)
	}
	fd := file.Fd()
	info, err := unix.GetsockoptTCPInfo(int(fd), unix.SOL_TCP, unix.TCP_INFO)
	return info
}
