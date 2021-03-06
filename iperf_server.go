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

func (test *iperf_test) server_listen() int{
	listen_addr := ":"
	listen_addr += strconv.Itoa(int(test.port))
	var err error
	test.listener, err = net.Listen("tcp", listen_addr)
	if err != nil {
		return -1
	}
	fmt.Printf("Server listening on %v\n", test.port)
	return 0
}

func (test *iperf_test) handleServerCtrlMsg() {
	buf := make([]byte, 4)	// only for ctrl state
	for {
		if n, err := test.ctrl_conn.Read(buf); err==nil{
			state := binary.LittleEndian.Uint32(buf[:])
			log.Debugf("Ctrl conn receive n = %v state = [%v]", n, state)
			//if err != nil {
			//	log.Errorf("Convert string to int failed. s = %v", string(buf[:n]))
			//	return
			//}
			test.state = uint(state)
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
		case TEST_START:
			break
		case TEST_END:
			log.Infof("Server Enter Test End state...")
			test.done = true
			if test.stats_callback != nil {
				test.stats_callback(test)
			}
			test.close_all_streams()

			/* exchange result mode */
			if test.set_send_state(IPERF_EXCHANGE_RESULT) < 0 {
				log.Errorf("set_send_state error")
				return
			}
			log.Infof("Server Enter Exchange Result state...")
			if test.exchange_results() < 0{
				log.Errorf("exchange result failed")
				return
			}

			/* display result mode */
			if test.set_send_state(IPERF_DISPLAY_RESULT) < 0 {
				log.Errorf("set_send_state error")
				return
			}
			log.Infof("Server Enter Display Result state...")
			if test.reporter_callback != nil {	// why call these again
				test.reporter_callback(test)
			}
			//if test.display_results() < 0 {
			//	log.Errorf("display result failed")
			//	return
			//}
			// on_test_finish undo
		case IPERF_DONE:
			test.state = IPERF_DONE
			log.Debugf("Server reach IPERF_DONE")
			test.ctrl_chan <- IPERF_DONE
			test.proto.teardown(test)
			return
		case CLIENT_TERMINATE:		//not used yet
			old_state := test.state
			test.state = IPERF_DISPLAY_RESULT
			test.reporter_callback(test)
			test.state = old_state

			test.close_all_streams()
			log.Infof("Client is terminated.")
			test.state = IPERF_DONE
			break
		default:
			log.Errorf("Unexpected situation with state = %v.", test.state)
			return
		}
	}
}

func (test *iperf_test) create_server_timer() int {
	now := time.Now()
	cd := TimerClientData{p: test}
	test.timer = timer_create(now, server_timer_proc, cd, (test.duration + 5) * 1000)	// convert sec to ms, add 5 sec to ensure client end first
	times := test.duration * 1000 / test.interval
	test.stats_ticker = ticker_create(now, server_stats_ticker_proc, cd, test.interval, times - 1)
	test.report_ticker = ticker_create(now, server_report_ticker_proc, cd, test.interval, times - 1)
	if test.timer.timer == nil || test.stats_ticker.ticker == nil || test.report_ticker.ticker == nil {
		log.Error("timer create failed.")
	}
	return 0
}

func server_timer_proc(data TimerClientData, now time.Time){
	log.Debugf("Enter server_timer_proc")
	test := data.p.(*iperf_test)
	if test.done {
		return
	}
	test.done = true
	// close all streams
	for _, sp := range test.streams{
		sp.conn.Close()
	}
	test.timer.done <- true
	//test.ctrl_conn.Close()		//  ctrl conn should be closed at last
	//log.Infof("Server exceed duration. Close control connection.")
}

func server_stats_ticker_proc(data TimerClientData, now time.Time){
	test := data.p.(*iperf_test)
	if test.done{
		return
	}
	if test.stats_callback != nil{
		test.stats_callback(test)
	}
}

func server_report_ticker_proc(data TimerClientData, now time.Time){
	test := data.p.(*iperf_test)
	if test.done{
		return
	}
	if test.reporter_callback != nil{
		test.reporter_callback(test)
	}
}

func (test *iperf_test) create_server_omit_timer() int {
	// undo, depend on which kind of timer
	return 0
}


func (test *iperf_test) run_server() int{
	log.Debugf("Enter run_server")

	if test.server_listen() < 0 {
		log.Error("Listen failed")
		return -1
	}
	test.state = IPERF_START
	log.Info("Enter Iperf start state...")
	// start
	conn, err := test.listener.Accept()
	if err != nil {
		log.Error("Accept failed")
		return -2
	}
	test.ctrl_conn = conn
	fmt.Printf("Accept connection from client: %v\n", conn.RemoteAddr())
	// exchange params
	if test.set_send_state(IPERF_EXCHANGE_PARAMS) < 0 {
		log.Error("set_send_state error.")
		return -3
	}
	log.Info("Enter Exchange Params state...")

	if test.exchange_params() < 0 {
		log.Error("exchange params failed.")
		return -3
	}

	go test.handleServerCtrlMsg()	// coroutine handle control msg

	if test.is_server == true {
		listener, err := test.proto.listen(test)
		if  err != nil {
			log.Error("proto listen error.")
			return -4
		}
		test.proto_listener = listener
	}

	// create streams
	if test.set_send_state(IPERF_CREATE_STREAM) < 0 {
		log.Error("set_send_state error.")
		return -3
	}
	log.Info("Enter Create Stream state...")

	var is_iperf_done bool = false
	for is_iperf_done != true{
		select {
		case state := <- test.ctrl_chan:
			log.Debugf("Ctrl channel receive state [%v]", state)
			if state == IPERF_DONE{
				return 0
			} else if state == IPERF_CREATE_STREAM {
				var stream_num uint = 0
				for stream_num < test.stream_num{
					proto_conn, err := test.proto.accept(test)
					if err != nil {
						log.Error("proto accept error.")
						return -4
					}
					stream_num ++
					var sp *iperf_stream
					if test.mode == IPERF_SENDER {
						sp = test.new_stream(proto_conn, SENDER_STREAM)
					} else {
						sp = test.new_stream(proto_conn, RECEIVER_STREAM)
					}

					if sp == nil {
						log.Error("Create new strema failed.")
						return -4
					}
					test.streams = append(test.streams, sp)
					log.Debugf("create new stream, stream_num = %v, target stream num = %v", stream_num, test.stream_num)
				}
				if stream_num == test.stream_num {
					if test.set_send_state(TEST_START) != 0{
						log.Errorf("set_send_state error")
						return -5
					}
					log.Info("Enter Test Start state...")
					if test.init_test() < 0 {
						log.Errorf("Init test failed.")
						return -5
					}
					if test.create_server_timer() < 0 {
						log.Errorf("Create Server timer failed.")
						return -6
					}
					if test.create_server_omit_timer() < 0{
						log.Errorf("Create Server Omit timer failed.")
						return -7
					}
					if test.mode == IPERF_SENDER{
						if rtn := test.create_sender_ticker(); rtn < 0 {
							log.Errorf("create_sender_ticker failed. rtn = %v", rtn)
							return -7
						}
					}
					if test.set_send_state(TEST_RUNNING) != 0{
						log.Errorf("set_send_state error")
						return -8
					}
				}
			} else if state == TEST_RUNNING{
				// Regular mode. Server receives.
				log.Info("Enter Test Running state...")
				for i, sp := range test.streams{
					if sp.role == SENDER_STREAM {
						go sp.iperf_send(test)
						log.Infof("Server Stream %v start sending.", i)
					} else {
						go sp.iperf_recv(test)
						log.Infof("Server Stream %v start receiving.", i)
					}
				}
				log.Info("Server all streams start...")
			} else if state == TEST_END {
				continue
			} else if state == IPERF_DONE{
				is_iperf_done = true
			} else {
				log.Debugf("Channel Unhandle state [%v]", state)
			}
		}
	}
	log.Debugf("Server side done.")
	return 0
}


