package main

import (
	"io"
	"net"
	"os"
	"runtime"
	"strconv"
	"time"
)

type tcp_proto struct{
}

func (tcp *tcp_proto) name() string{
	return TCP_NAME
}

func (tcp *tcp_proto) accept(test *iperf_test) (net.Conn, error){
	log.Debugf("Enter TCP accept")
	conn, err := test.proto_listener.Accept()
	if err != nil{
		return nil, err
	}
	return conn, err
}

func (tcp *tcp_proto) listen(test *iperf_test) (net.Listener, error){
	log.Debugf("Enter TCP listen")
	// continue use the formal listener
	return test.listener, nil
}

func (tcp *tcp_proto) connect(test *iperf_test) (net.Conn, error){
	log.Debugf("Enter TCP connect")
	tcpAddr, err := net.ResolveTCPAddr("tcp4", test.addr + ":" + strconv.Itoa(int(test.port)))
	if err != nil {
		return nil, err
	}
	conn, err := net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		return nil, err
	}
	conn.SetDeadline(time.Now().Add(time.Duration(test.duration + 5)*time.Second))
	return conn, nil
}

func (tcp *tcp_proto) send(sp *iperf_stream) int{
	// write is blocking
	n, err := sp.conn.(*net.TCPConn).Write(sp.buffer)
	if err != nil {
		if serr, ok := err.(*net.OpError); ok{
			log.Debugf("tcp conn already close = %v", serr)
			return -1
		} else if err == os.ErrClosed || err == io.ErrClosedPipe{
			log.Debugf("send tcp socket close.")
			return -1
		}
		log.Errorf("tcp write err = %T %v",err, err)
		return -2
	}
	if n < 0 {
		return n
	}
	sp.result.bytes_sent += uint64(n)
	sp.result.bytes_sent_this_interval += uint64(n)
	//log.Debugf("tcp send %v bytes of total %v", n, sp.result.bytes_sent)
	return n
}

func (tcp *tcp_proto) recv(sp *iperf_stream) int{
	// recv is blocking
	n, err := sp.conn.(*net.TCPConn).Read(sp.buffer)

	if err != nil {
		if serr, ok := err.(*net.OpError); ok{
			log.Debugf("tcp conn already close = %v", serr)
			return -1
		} else if err == io.EOF || err == os.ErrClosed || err == io.ErrClosedPipe{
			log.Debugf("recv tcp socket close. EOF")
			return -1
		}
		log.Errorf("tcp recv err = %T %v",err, err)
		return -2
	}
	if n < 0 {
		return n
	}
	if sp.test.state == TEST_RUNNING {
		sp.result.bytes_received += uint64(n)
		sp.result.bytes_received_this_interval += uint64(n)
	}
	//log.Debugf("tcp recv %v bytes of total %v", n, sp.result.bytes_received)
	return n
}

func (tcp *tcp_proto) init(test *iperf_test) int{
	if test.no_delay == true {
		for _, sp := range test.streams{
			err := sp.conn.(*net.TCPConn).SetNoDelay(test.no_delay)
			if err != nil {
				return -1
			}
		}
	}
	return 0
}

func (tcp *tcp_proto) stats_callback(test *iperf_test, sp *iperf_stream, temp_result *iperf_interval_results) int {
	if test.proto.name() == TCP_NAME && has_tcpInfo(){		// only linux has tcp info
		rp := sp.result
		save_tcpInfo(sp, temp_result)
		total_retrans := temp_result.interval_retrans  // get the temporarily result
		temp_result.interval_retrans = total_retrans - rp.stream_prev_total_retrans
		rp.stream_retrans += temp_result.interval_retrans
		rp.stream_prev_total_retrans = total_retrans
		if rp.stream_min_rtt == 0 || temp_result.rtt < rp.stream_min_rtt {
			rp.stream_min_rtt = temp_result.rtt
		}
		if rp.stream_max_rtt == 0 || temp_result.rtt > rp.stream_max_rtt {
			rp.stream_max_rtt = temp_result.rtt
		}
		rp.stream_sum_rtt += temp_result.rtt
		rp.stream_cnt_rtt ++
	}
	return 0
}

func (tcp *tcp_proto) teardown(test *iperf_test) int{
	return 0
}

func has_tcpInfo() bool{
	switch runtime.GOOS {
	case "windows":
		return false
	case "linux":
		return true
	default:
		return false
	}
}
