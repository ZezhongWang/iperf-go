package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"time"
)

func (test *iperf_test) create_streams() int {
	for i := uint(0) ; i < test.stream_num ; i ++ {
		conn, err := test.proto.connect(test)
		if err != nil {
			log.Errorf("Connect failed. err = %v", err)
			return -1
		}
		var sp *iperf_stream
		if test.mode == IPERF_SENDER {
			sp = test.new_stream(conn, SENDER_STREAM)
		} else {
			sp = test.new_stream(conn, RECEIVER_STREAM)
		}
		test.streams = append(test.streams, sp)
	}
	return 0
}

func (test *iperf_test) create_client_timer() int {
	now := time.Now()
	cd := TimerClientData{p: test}
	test.timer = timer_create(now, client_timer_proc, cd, test.duration * 1000 )	// convert sec to ms
	times := test.duration * 1000 / test.interval
	test.stats_ticker = ticker_create(now, client_stats_ticker_proc, cd, test.interval, times - 1)
	test.report_ticker = ticker_create(now, client_report_ticker_proc, cd, test.interval, times - 1)
	if test.timer.timer == nil || test.stats_ticker.ticker == nil || test.report_ticker.ticker == nil {
		log.Error("timer create failed.")
	}
	return 0
}

func client_timer_proc(data TimerClientData, now time.Time){
	log.Debugf("Enter client_timer_proc")
	test := data.p.(*iperf_test)
	test.timer.done <- true
	test.done = true	// will end send/recv in iperf_send/iperf_recv, and then triggered TEST_END
	test.timer.timer = nil
}

func client_stats_ticker_proc(data TimerClientData, now time.Time){
	test := data.p.(*iperf_test)
	if test.done{
		return
	}
	if test.stats_callback != nil{
		test.stats_callback(test)
	}
}

func client_report_ticker_proc(data TimerClientData, now time.Time){
	test := data.p.(*iperf_test)
	if test.done{
		return
	}
	if test.reporter_callback != nil{
		test.reporter_callback(test)
	}
}

func (test *iperf_test) create_client_omit_timer() int {
	// undo, depend on which kind of timer
	return 0
}

func send_ticker_proc(data TimerClientData, now time.Time){
	sp := data.p.(*iperf_stream)
	sp.test.check_throttle(sp, now)
}

func (test *iperf_test) client_end(){
	log.Debugf("Enter client_end")
	for _, sp := range test.streams{
		sp.conn.Close()
	}
	if test.reporter_callback != nil {	// call only after exchange_result finish
		test.reporter_callback(test)
	}
	test.proto.teardown(test)
	if test.set_send_state(IPERF_DONE) < 0 {
		log.Errorf("set_send_state failed")
	}

	log.Infof("Client Enter IPerf Done...")
	if test.ctrl_conn != nil {
		test.ctrl_conn.Close()
	}
}

func (test *iperf_test) handleClientCtrlMsg() {
	buf := make([]byte, 4)
	for {
		if n, err := test.ctrl_conn.Read(buf); err==nil{
			state := binary.LittleEndian.Uint32(buf[:])
			log.Debugf("Client Ctrl conn receive n = %v state = [%v]", n, state)
			//state, err := strconv.Atoi(string(buf[:n]))

			//if err != nil {
			//	log.Errorf("Convert string to int failed. s = %v", string(buf[:n]))
			//	return
			//}
			test.state = uint(state)
			log.Infof("Client Enter %v state...", test.state)
		} else {
			if serr, ok := err.(*net.OpError); ok{
				log.Info("Client control connection close. err = %T %v", serr, serr)
				test.ctrl_conn.Close()
			} else if err == os.ErrClosed || err == io.ErrClosedPipe || err == io.EOF{
				log.Info("Client control connection close. err = %T %v", serr, serr)
			} else {
				log.Errorf("ctrl_conn read failed. err=%T, %v", err, err)
				test.ctrl_conn.Close()
			}
			return
		}

		switch test.state {
		case IPERF_EXCHANGE_PARAMS:
			if rtn := test.exchange_params(); rtn < 0 {
				log.Errorf("exchange_params failed. rtn = %v", rtn)
				return
			}
		case IPERF_CREATE_STREAM:
			if rtn := test.create_streams(); rtn < 0 {
				log.Errorf("create_streams failed. rtn = %v", rtn)
				return
			}
		case TEST_START:
			// handle test start
			if rtn := test.init_test(); rtn < 0 {
				log.Errorf("init_test failed. rtn = %v", rtn)
				return
			}
			if rtn := test.create_client_timer(); rtn < 0 {
				log.Errorf("create_client_timer failed. rtn = %v", rtn)
				return
			}
			if rtn := test.create_client_omit_timer(); rtn < 0 {
				log.Errorf("create_client_omit_timer failed. rtn = %v", rtn)
				return
			}
			if test.mode == IPERF_SENDER{
				if rtn := test.create_sender_ticker(); rtn < 0 {
					log.Errorf("create_sender_ticker failed. rtn = %v", rtn)
					return
				}
			}
		case TEST_RUNNING:
			test.ctrl_chan <- TEST_RUNNING
			break
		case IPERF_EXCHANGE_RESULT:
			if rtn := test.exchange_results(); rtn < 0 {
				log.Errorf("exchange_results failed. rtn = %v", rtn)
				return
			}
		case IPERF_DISPLAY_RESULT:
			test.client_end()
		case IPERF_DONE:
			break
		case SERVER_TERMINATE:
			old_state := test.state
			test.state = IPERF_DISPLAY_RESULT
			test.reporter_callback(test)
			test.state = old_state
		default:
			log.Errorf("Unexpected situation with state = %v.", test.state)
			return
		}
	}
}

func (test *iperf_test) ConnectServer() int{
	tcpAddr, err := net.ResolveTCPAddr("tcp4", test.addr + ":" + strconv.Itoa(int(test.port)))
	if err != nil {
		log.Errorf("Resolve TCP Addr failed. err = %v, addr = %v", err, test.addr + strconv.Itoa(int(test.port)))
		return -1
	}
	conn, err := net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		log.Errorf("Connect TCP Addr failed. err = %v, addr = %v", err, test.addr + strconv.Itoa(int(test.port)))
		return -1
	}
	test.ctrl_conn = conn
	fmt.Printf("Connect to server %v succeed.\n", test.addr + ":" + strconv.Itoa(int(test.port)))
	return 0
}
func (test *iperf_test) run_client() int{

	rtn := test.ConnectServer()
	if rtn < 0 {
		log.Errorf("ConnectServer failed")
		return -1
	}

	go test.handleClientCtrlMsg()

	var is_iperf_done bool = false
	var test_end_num uint = 0
	for is_iperf_done != true {
		select {
		case state := <-test.ctrl_chan:
			if state == TEST_RUNNING {
				// set non-block for non-UDP test. unfinished
				// Regular mode. Client sends.
				log.Info("Client enter Test Running state...")
					for i, sp := range test.streams{
						if sp.role == SENDER_STREAM {
							go sp.iperf_send(test)
							log.Infof("Client Stream %v start sending.", i)
						} else {
							go sp.iperf_recv(test)
							log.Infof("Client Stream %v start receiving.", i)
						}
					}
				log.Info("Create all streams finish...")
			} else if state == TEST_END {
				test_end_num ++
				if test_end_num < test.stream_num || test_end_num == test.stream_num + 1{ 	// redundant TEST_END signal generate by set_send_state
					continue
				} else if test_end_num > test.stream_num + 1{
					log.Errorf("Receive more TEST_END signal than expected")
					return -1
				}
				log.Infof("Client all Stream closed.")
				// test_end_num == test.stream_num. all the stream send TEST_END signal
				test.done = true
				if test.set_send_state(TEST_END) < 0 {
					log.Errorf("set_send_state failed. %v", TEST_END)
					return -1
				}
				if test.stats_callback != nil {
					test.stats_callback(test)
				}
				log.Info("Client Enter Test End State.")
			} else if state == IPERF_DONE {
				is_iperf_done = true
			} else {
				log.Debugf("Channel Unhandle state [%v]", state)
			}
		}
	}
	return 0
}
