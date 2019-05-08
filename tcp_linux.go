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
	rp.rto = uint(info.Rto)
	rp.interval_retrans = uint(info.Total_retrans) // temporarily store
	//sp.test.r info.Retrans
	//PrintTCPInfo(info)
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

func PrintTCPInfo(info *unix.TCPInfo){
	fmt.Printf("TcpInfo: rcv_rtt:%v\trtt:%v\tretransmits:%v\trto:%v\tlost:%v\tretrans:%v\ttotal_retrans:%v\n",
		info.Rcv_rtt /* 作为接收端，测出的RTT值，单位为微秒*/, info.Rtt, info.Retransmits, info.Rto, info.Lost, info.Retrans /* 重传且未确认的数据段数 */ , info.Total_retrans /* 本连接的总重传个数 */)
}