package main

import (
	RUDP "../rudp-go"
	"encoding/binary"
	"fmt"
	"github.com/op/go-logging"
	"io"
	"net"
	"os"
	"strconv"
)


type rudp_proto struct{
}

func (rudp *rudp_proto) name() string{
	return RUDP_NAME
}

func (rudp *rudp_proto) accept(test *iperf_test) (net.Conn, error){
	log.Debugf("Enter RUDP accept")
	conn, err := test.proto_listener.Accept()
	if err != nil{
		return nil, err
	}
	buf := make([]byte, 4)
	n, err := conn.Read(buf)
	signal := binary.LittleEndian.Uint32(buf[:])
	if err != nil || n != 4 || signal != ACCEPT_SIGNAL{
		log.Errorf("RUDP Receive Unexpected signal")
	}
	log.Debugf("RUDP accept succeed. signal = %v", signal)
	return conn, nil
}

func (rudp *rudp_proto) listen(test *iperf_test) (net.Listener, error){
	listener, err := RUDP.ListenWithOptions(":" + strconv.Itoa(int(test.port)))
	listener.SetReadBuffer(int(test.setting.read_buf_size))			// all income conn share the same underline packet conn, the buffer should be large
	listener.SetWriteBuffer(int(test.setting.write_buf_size))

	if err != nil {
		return nil, err
	}
	return listener, nil
}

func (rudp *rudp_proto) connect(test *iperf_test) (net.Conn, error){
	conn, err := RUDP.ConnectRUDP(test.addr + ":" + strconv.Itoa(int(test.port)))
	if err != nil {
		return nil, err
	}
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, ACCEPT_SIGNAL)
	n, err := conn.Write(buf)
	if err != nil || n != 4 {
		log.Errorf("RUDP send accept signal failed")
	}
	log.Debugf("RUDP connect succeed.")
	return conn, nil
}

func (rudp *rudp_proto) send(sp *iperf_stream) int{
	n, err := sp.conn.(*RUDP.RUDPSession).Write(sp.buffer)
	if err != nil {
		if serr, ok := err.(*net.OpError); ok{
			log.Debugf("rudp conn already close = %v", serr)
			return -1
		} else if err.Error() == "broken pipe"{
			log.Debugf("rudp conn already close = %v", err.Error())
			return -1
		} else if err == os.ErrClosed || err == io.ErrClosedPipe{
			log.Debugf("send rudp socket close.")
			return -1
		}
		log.Errorf("rudp write err = %T %v",err, err)
		return -2
	}
	if n < 0 {
		log.Errorf("rudp write err. n = %v" ,n)
		return n
	}
	sp.result.bytes_sent += uint64(n)
	sp.result.bytes_sent_this_interval += uint64(n)
	//log.Debugf("RUDP send %v bytes of total %v", n, sp.result.bytes_sent)
	return n
}

func (rudp *rudp_proto) recv(sp *iperf_stream) int{
	// recv is blocking
	n, err := sp.conn.(*RUDP.RUDPSession).Read(sp.buffer)

	if err != nil {
		if serr, ok := err.(*net.OpError); ok{
			log.Debugf("rudp conn already close = %v", serr)
			return -1
		} else if err.Error() == "broken pipe"{
			log.Debugf("rudp conn already close = %v", err.Error())
			return -1
		} else if err == io.EOF || err == os.ErrClosed || err == io.ErrClosedPipe{
			log.Debugf("recv rudp socket close. EOF")
			return -1
		}
		log.Errorf("rudp recv err = %T %v",err, err)
		return -2
	}
	if n < 0 {
		return n
	}
	if sp.test.state == TEST_RUNNING {
		sp.result.bytes_received += uint64(n)
		sp.result.bytes_received_this_interval += uint64(n)
	}
	//log.Debugf("RUDP recv %v bytes of total %v", n, sp.result.bytes_received)
	return n
}

func (rudp *rudp_proto) init(test *iperf_test) int{
	for _, sp := range test.streams {
		sp.conn.(*RUDP.RUDPSession).SetReadBuffer(int(test.setting.read_buf_size))
		sp.conn.(*RUDP.RUDPSession).SetWriteBuffer(int(test.setting.write_buf_size))
		sp.conn.(*RUDP.RUDPSession).SetWindowSize(int(test.setting.snd_wnd), int(test.setting.rcv_wnd))
		sp.conn.(*RUDP.RUDPSession).SetStreamMode(true)
		var no_delay, resend, nc int
		if test.no_delay {
			no_delay = 1
		} else {
			no_delay = 0
		}
		if test.setting.no_cong {
			nc = 1
		} else {
			nc = 0
		}
		resend = int(test.setting.fast_resend)
		sp.conn.(*RUDP.RUDPSession).SetNoDelay(no_delay, int(test.setting.flush_interval), resend, nc)
	}
	return 0
}

func (rudp *rudp_proto) stats_callback(test *iperf_test, sp *iperf_stream, temp_result *iperf_interval_results) int {
	rp := sp.result
	total_retrans := uint(RUDP.DefaultSnmp.RetransSegs)
	total_lost := uint(RUDP.DefaultSnmp.LostSegs)
	total_early_retrans := uint(RUDP.DefaultSnmp.EarlyRetransSegs)
	total_fast_retrans := uint(RUDP.DefaultSnmp.FastRetransSegs)
	// retrans
	temp_result.interval_retrans = total_retrans - rp.stream_prev_total_retrans
	rp.stream_retrans += temp_result.interval_retrans
	rp.stream_prev_total_retrans = total_retrans
	// lost
	temp_result.interval_lost = total_lost - rp.stream_prev_total_lost
	rp.stream_lost += temp_result.interval_lost
	rp.stream_prev_total_lost = total_lost
	// early retrans
	temp_result.interval_early_retrans = total_early_retrans - rp.stream_prev_total_early_retrans
	rp.stream_early_retrans += temp_result.interval_early_retrans
	rp.stream_prev_total_early_retrans = total_early_retrans
	// fast retrans
	temp_result.interval_fast_retrans = total_fast_retrans - rp.stream_prev_total_fast_retrans
	rp.stream_fast_retrans += temp_result.interval_fast_retrans
	rp.stream_prev_total_fast_retrans = total_fast_retrans

	temp_result.rto = sp.conn.(*RUDP.RUDPSession).GetRTO() * 1000
	temp_result.rtt = sp.conn.(*RUDP.RUDPSession).GetRTT() * 1000		// ms to micro sec
	if rp.stream_min_rtt == 0 || temp_result.rtt < rp.stream_min_rtt {
		rp.stream_min_rtt = temp_result.rtt
	}
	if rp.stream_max_rtt == 0 || temp_result.rtt > rp.stream_max_rtt {
		rp.stream_max_rtt = temp_result.rtt
	}
	rp.stream_sum_rtt += temp_result.rtt
	rp.stream_cnt_rtt ++
	return 0
}

func (rudp *rudp_proto)teardown(test *iperf_test) int{
	if logging.GetLevel("rudp") == logging.INFO ||
		logging.GetLevel("rudp") == logging.DEBUG{
		header := RUDP.DefaultSnmp.Header()
		slices := RUDP.DefaultSnmp.ToSlice()
		for k := range header{
			fmt.Printf("%s: %v\t", header[k], slices[k])
		}
		fmt.Printf("\n")
		if test.setting.no_cong == false {
			RUDP.PrintTracker()
		}
	}
	return 0
}