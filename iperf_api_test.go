package main

import (
	"encoding/binary"
	"github.com/op/go-logging"
	"testing"
	"time"
	//"github.com/gotestyourself/gotest.tools/assert"
	"gotest.tools/assert"
	//"github.com/stretchr/testify/assert"

)

const portServer = 5021
const addrServer = "127.0.0.1:5021"
const addrClient = "127.0.0.1"
var server_test, client_test *iperf_test

func init(){

	logging.SetLevel(logging.ERROR, "iperf")
	logging.SetLevel(logging.ERROR, "rudp")
	/* log settting */

	server_test = new_iperf_test()
	client_test = new_iperf_test()
	server_test.init()
	client_test.init()

	server_test.is_server = true
	server_test.port = portServer

	client_test.is_server = false
	client_test.port = portServer
	client_test.addr = addrClient

	client_test.interval = 1000 	// 1000 ms
	client_test.duration = 5		// 5 s for test
	client_test.stream_num = 1		// 1 stream only
	client_test.set_test_reverse(false)

	//TCPSetting()
	RUDPSetting()
	//KCPSetting()

	//client_test.setting.burst = true
	go server_test.run_server()
	time.Sleep(time.Second)
}

func TCPSetting(){
	client_test.set_protocol(TCP_NAME)
	client_test.no_delay = true
	client_test.setting.blksize = DEFAULT_TCP_BLKSIZE
	client_test.setting.burst = false
	client_test.setting.rate = 1024*1024*1024*1024		// b/s
	client_test.setting.pacing_time = 100		//ms
}

func RUDPSetting(){
	client_test.set_protocol(RUDP_NAME)
	client_test.no_delay = false
	client_test.setting.blksize = DEFAULT_RUDP_BLKSIZE
	client_test.setting.burst = true
	client_test.setting.no_cong = false			// false for BBR control
	client_test.setting.snd_wnd = 10
	client_test.setting.rcv_wnd = 1024
	client_test.setting.read_buf_size = DEFAULT_READ_BUF_SIZE
	client_test.setting.write_buf_size = DEFAULT_WRITE_BUF_SIZE
	client_test.setting.flush_interval = DEFAULT_FLUSH_INTERVAL
	client_test.setting.data_shards = 3
	client_test.setting.parity_shards = 1
}

func KCPSetting(){
	client_test.set_protocol(KCP_NAME)
	client_test.no_delay = false
	client_test.setting.blksize = DEFAULT_RUDP_BLKSIZE
	client_test.setting.burst = true
	client_test.setting.no_cong = true			// false for BBR control
	client_test.setting.snd_wnd = 512
	client_test.setting.rcv_wnd = 1024
	client_test.setting.read_buf_size = DEFAULT_READ_BUF_SIZE
	client_test.setting.write_buf_size = DEFAULT_WRITE_BUF_SIZE
	client_test.setting.flush_interval = DEFAULT_FLUSH_INTERVAL
}

func RecvCheckState(t *testing.T, state int) int {
	buf := make([]byte, 4)
	if n, err := client_test.ctrl_conn.Read(buf); err == nil {
		s := binary.LittleEndian.Uint32(buf[:])
		log.Debugf("Ctrl conn receive n = %v state = [%v]", n, s)
		//s, err := strconv.Atoi(string(buf[:n]))
		if s != uint32(state){
			log.Errorf("recv state[%v] != expected state[%v]", s, state)
			t.FailNow()
			return -1
		}
		client_test.state = uint(state)
		log.Infof("Client Enter %v state", client_test.state)
	}
	return 0
}

func CreateStreams(t *testing.T) int{
	if rtn := client_test.create_streams(); rtn < 0 {
		log.Errorf("create_streams failed. rtn = %v", rtn)
		return -1
	}
	// check client state
	assert.Equal(t, uint(len(client_test.streams)), client_test.stream_num)
	for _, sp := range(client_test.streams){
		assert.Equal(t, sp.test, client_test)
		if client_test.mode == IPERF_SENDER {
			assert.Equal(t, sp.role, SENDER_STREAM)
		} else{
			assert.Equal(t, sp.role, RECEIVER_STREAM)
		}
		assert.Assert(t, sp.result != nil)
		assert.Equal(t, sp.can_send, false)		// set true after create_send_timer
		assert.Assert(t, sp.conn != nil)
		assert.Assert(t, sp.send_ticker.ticker == nil)		// ticker haven't been created yet
	}
	time.Sleep(time.Millisecond * 10)		// ensure server side has created all the streams
	// check server state
	assert.Equal(t, uint(len(server_test.streams)), client_test.stream_num)
	for _, sp := range(server_test.streams){
		assert.Equal(t, sp.test, server_test)
		if server_test.mode == IPERF_SENDER {
			assert.Equal(t, sp.role, SENDER_STREAM)
		} else {
			assert.Equal(t, sp.role, RECEIVER_STREAM)
		}
		assert.Assert(t, sp.result != nil)
		if server_test.mode == IPERF_SENDER {
			assert.Equal(t, sp.can_send, true)
			if client_test.setting.burst == true {
				assert.Assert(t, sp.send_ticker.ticker == nil)
			} else {
				assert.Assert(t, sp.send_ticker.ticker != nil)
			}
		} else {
			assert.Equal(t, sp.can_send, false)
			assert.Assert(t, sp.send_ticker.ticker == nil)
		}
		assert.Assert(t, sp.conn != nil)

	}
	return 0
}

func handleTestStart(t *testing.T) int{
	if rtn := client_test.init_test(); rtn < 0 {
		log.Errorf("init_test failed. rtn = %v", rtn)
		return -1
	}
	if rtn := client_test.create_client_timer(); rtn < 0 {
		log.Errorf("create_client_timer failed. rtn = %v", rtn)
		return -1
	}
	if rtn := client_test.create_client_omit_timer(); rtn < 0 {
		log.Errorf("create_client_omit_timer failed. rtn = %v", rtn)
		return -1
	}
	if client_test.mode == IPERF_SENDER{
		if rtn := client_test.create_sender_ticker(); rtn < 0 {
			log.Errorf("create_client_send_timer failed. rtn = %v", rtn)
			return -1
		}
	}

	// check client
	for _, sp := range client_test.streams{
		assert.Assert(t, sp.result.start_time.Before(time.Now().Add(time.Duration(time.Millisecond))))
		assert.Assert(t, sp.test.timer.timer != nil)
		assert.Assert(t, sp.test.stats_ticker.ticker != nil)
		assert.Assert(t, sp.test.report_ticker.ticker != nil)

		if client_test.mode == IPERF_SENDER {
			assert.Equal(t, sp.can_send, true)
			if client_test.setting.burst == true {
				assert.Assert(t, sp.send_ticker.ticker == nil)
			} else {
				assert.Assert(t, sp.send_ticker.ticker != nil)
			}
		} else {
			assert.Equal(t, sp.can_send, false)
			assert.Assert(t, sp.send_ticker.ticker == nil)
		}
	}

	// check server, should finish test_start process and enter test_running now
	for _, sp := range server_test.streams{
		assert.Assert(t, sp.result.start_time.Before(time.Now().Add(time.Duration(time.Millisecond))))
		assert.Assert(t, sp.test.timer.timer != nil)
		assert.Assert(t, sp.test.stats_ticker.ticker != nil)
		assert.Assert(t, sp.test.report_ticker.ticker != nil)
		assert.Equal(t, sp.test.state, uint(TEST_RUNNING))
	}

	return 0
}

func handleTestRunning(t *testing.T) int{
	log.Info("Client enter Test Running state...")
	for i, sp := range client_test.streams{
		if client_test.mode == IPERF_SENDER {
			go sp.iperf_send(client_test)
			log.Infof("Stream %v start sending.", i)
		} else {
			go sp.iperf_recv(client_test)
			log.Infof("Stream %v start receiving.", i)
		}
	}
	log.Info("Client all Stream start. Wait for finish...")
	// wait for send/write end (triggered by timer)
	//for {
	//	if client_test.done {
	//		time.Sleep(time.Millisecond)
	//		break
	//	}
	//}
	for i := 0; i < int(client_test.stream_num); i++ {
		s := <- client_test.ctrl_chan
		assert.Equal(t, s, uint(TEST_END))
	}
	log.Infof("Client All Send Stream closed.")
	client_test.done = true
	if client_test.stats_callback != nil {
		client_test.stats_callback(client_test)
	}
	if client_test.set_send_state(TEST_END) < 0 {
		log.Errorf("set_send_state failed. %v", TEST_END)
		t.FailNow()
	}
	// check client
	assert.Equal(t, client_test.done, true)
	assert.Assert(t, client_test.timer.timer == nil)
	assert.Equal(t, client_test.state, uint(TEST_END))
	var total_bytes uint64
	for _, sp := range client_test.streams  {
		if client_test.mode == IPERF_SENDER{
			total_bytes += sp.result.bytes_sent
		} else {
			total_bytes += sp.result.bytes_received
		}
	}
	if client_test.mode == IPERF_SENDER{
		assert.Equal(t, client_test.bytes_sent, total_bytes)
		assert.Equal(t, client_test.bytes_received, uint64(0))
	} else {
		assert.Equal(t, client_test.bytes_received, total_bytes)
		assert.Equal(t, client_test.bytes_sent, uint64(0))
	}



	time.Sleep(time.Millisecond*10)	// ensure server change state
	// check server
	assert.Equal(t, server_test.done, true)
	assert.Equal(t, server_test.state, uint(IPERF_EXCHANGE_RESULT))
	absolute_bytes_diff := int64(server_test.bytes_received) - int64(client_test.bytes_sent)
	if absolute_bytes_diff < 0{
		 absolute_bytes_diff = 0 - absolute_bytes_diff
	}
	if float64(absolute_bytes_diff) / float64(client_test.bytes_sent) > 0.01 {	// if bytes difference larger than 1%
		t.FailNow()
	}
	//assert.Equal(t, server_test.bytes_received, client_test.bytes_sent)
	//assert.Equal(t, server_test.blocks_received, client_test.blocks_sent)		// block num not always same
	total_bytes = 0
	for _, sp := range server_test.streams  {
		if server_test.mode == IPERF_SENDER{
			total_bytes += sp.result.bytes_sent
		} else {
			total_bytes += sp.result.bytes_received
		}
	}
	if server_test.mode == IPERF_SENDER {
		assert.Equal(t, server_test.bytes_sent, total_bytes)
		assert.Equal(t, server_test.bytes_received, uint64(0))
	} else {
		assert.Equal(t, server_test.bytes_received, total_bytes)
		assert.Equal(t, server_test.bytes_sent, uint64(0))
	}
	return 0
}

func handleExchangeResult(t *testing.T) int{
	if rtn := client_test.exchange_results(); rtn < 0 {
		log.Errorf("exchange_results failed. rtn = %v", rtn)
		return -1
	}
	// check client
	assert.Equal(t, client_test.done, true)
	for i, sp := range client_test.streams  {
		ssp := server_test.streams[i]
		assert.Equal(t, sp.result.bytes_received, ssp.result.bytes_received)
		assert.Equal(t, sp.result.bytes_sent, ssp.result.bytes_sent)
	}
	// check server
	assert.Equal(t, server_test.state, uint(IPERF_DISPLAY_RESULT))
	return 0
}
/*
	Test case can only be run one by one
 */

 /*
func TestCtrlConnect(t *testing.T){
	if rtn := client_test.ConnectServer(); rtn < 0 {
		t.FailNow()
	}
	RecvCheckState(t, IPERF_EXCHANGE_PARAMS)
	if err := client_test.ctrl_conn.Close(); err != nil {
		log.Errorf("close ctrl_conn failed.")
		t.FailNow()
	}
	if err := server_test.ctrl_conn.Close(); err != nil {
		log.Errorf("close ctrl_conn failed.")
		t.FailNow()
	}
}

func TestExchangeParams(t *testing.T){
	if rtn := client_test.ConnectServer(); rtn < 0 {
		t.FailNow()
	}
	RecvCheckState(t, IPERF_EXCHANGE_PARAMS)
	if rtn := client_test.exchange_params(); rtn < 0 {
		t.FailNow()
	}

	time.Sleep(time.Second)
	assert.Equal(t, server_test.proto.name(), client_test.proto.name())
	assert.Equal(t, server_test.stream_num, client_test.stream_num)
	assert.Equal(t, server_test.duration, client_test.duration)
	assert.Equal(t, server_test.interval, client_test.interval)
	assert.Equal(t, server_test.no_delay, client_test.no_delay)
}

func TestCreateOneStream(t *testing.T){
	// create only one stream
	if rtn := client_test.ConnectServer(); rtn < 0 {
		t.FailNow()
	}
	RecvCheckState(t, IPERF_EXCHANGE_PARAMS)
	if rtn := client_test.exchange_params(); rtn < 0 {
		t.FailNow()
	}
	RecvCheckState(t, IPERF_CREATE_STREAM)
	CreateStreams(t)
}

func TestCreateMultiStreams(t *testing.T){
	// create multi streams
	if rtn := client_test.ConnectServer(); rtn < 0 {
		t.FailNow()
	}
	RecvCheckState(t, IPERF_EXCHANGE_PARAMS)
	client_test.stream_num = 5	// change stream_num before exchange params
	if rtn := client_test.exchange_params(); rtn < 0 {
		t.FailNow()
	}
	RecvCheckState(t, IPERF_CREATE_STREAM)
	if rtn := CreateStreams(t); rtn < 0{
		t.FailNow()
	}
}

func TestTestStart(t *testing.T){
	if rtn := client_test.ConnectServer(); rtn < 0 {
		t.FailNow()
	}
	RecvCheckState(t, IPERF_EXCHANGE_PARAMS)
	if rtn := client_test.exchange_params(); rtn < 0 {
		t.FailNow()
	}
	RecvCheckState(t, IPERF_CREATE_STREAM)
	if rtn := CreateStreams(t); rtn < 0{
		t.FailNow()
	}
	RecvCheckState(t, TEST_START)
	if rtn := handleTestStart(t); rtn < 0{
		t.FailNow()
	}
	RecvCheckState(t, TEST_RUNNING)
}

func TestTestRunning(t *testing.T){
	if rtn := client_test.ConnectServer(); rtn < 0 {
		t.FailNow()
	}
	RecvCheckState(t, IPERF_EXCHANGE_PARAMS)
	client_test.stream_num = 2
	if rtn := client_test.exchange_params(); rtn < 0 {
		t.FailNow()
	}
	RecvCheckState(t, IPERF_CREATE_STREAM)
	if rtn := CreateStreams(t); rtn < 0{
		t.FailNow()
	}
	RecvCheckState(t, TEST_START)
	if rtn := handleTestStart(t); rtn < 0{
		t.FailNow()
	}
	RecvCheckState(t, TEST_RUNNING)
	if handleTestRunning(t) < 0{
		t.FailNow()
	}
	RecvCheckState(t, IPERF_EXCHANGE_RESULT)
}

func TestExchangeResult(t *testing.T){
	if rtn := client_test.ConnectServer(); rtn < 0 {
		t.FailNow()
	}
	RecvCheckState(t, IPERF_EXCHANGE_PARAMS)
	client_test.stream_num = 2
	if rtn := client_test.exchange_params(); rtn < 0 {
		t.FailNow()
	}
	RecvCheckState(t, IPERF_CREATE_STREAM)
	if rtn := CreateStreams(t); rtn < 0{
		t.FailNow()
	}
	RecvCheckState(t, TEST_START)
	if rtn := handleTestStart(t); rtn < 0{
		t.FailNow()
	}
	RecvCheckState(t, TEST_RUNNING)
	if handleTestRunning(t) < 0{
		t.FailNow()
	}
	RecvCheckState(t, IPERF_EXCHANGE_RESULT)
	if handleExchangeResult(t) < 0 {
		t.FailNow()
	}
	RecvCheckState(t, IPERF_DISPLAY_RESULT)
}
*/

func TestDisplayResult(t *testing.T){
	if rtn := client_test.ConnectServer(); rtn < 0 {
		t.FailNow()
	}
	RecvCheckState(t, IPERF_EXCHANGE_PARAMS)
	//client_test.stream_num = 2
	if rtn := client_test.exchange_params(); rtn < 0 {
		t.FailNow()
	}
	RecvCheckState(t, IPERF_CREATE_STREAM)
	if rtn := CreateStreams(t); rtn < 0{
		t.FailNow()
	}
	RecvCheckState(t, TEST_START)
	if rtn := handleTestStart(t); rtn < 0{
		t.FailNow()
	}
	RecvCheckState(t, TEST_RUNNING)
	if handleTestRunning(t) < 0{
		t.FailNow()
	}
	RecvCheckState(t, IPERF_EXCHANGE_RESULT)
	if handleExchangeResult(t) < 0 {
		t.FailNow()
	}
	RecvCheckState(t, IPERF_DISPLAY_RESULT)

	client_test.client_end()

	time.Sleep(time.Millisecond*10)		// wait for server
	assert.Equal(t, client_test.state, uint(IPERF_DONE))
	assert.Equal(t, server_test.state, uint(IPERF_DONE))
	// check output with your own eyes

	time.Sleep(time.Second*5)		// wait for server
}
