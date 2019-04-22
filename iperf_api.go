package main

import (
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"time"
)

func new_iperf_test() (test*iperf_test){
	test = new(iperf_test)
	test.ctrl_chan = make(chan uint, 5)
	test.setting = new(iperf_setting)
	test.reporter_callback = iperf_reporter_callback
	test.stats_callback = iperf_stats_callback
	test.chStats = make(chan bool, 1)
	return
}

func (test *iperf_test) set_protocol (proto_name string) int{
	for _, proto := range test.protocols{
		if proto_name == proto.name(){
			test.proto = proto
			return 0
		}
	}
	return -1
}

func (test *iperf_test) set_send_state(state uint) int{
	test.state = state
	test.ctrl_chan <- test.state
	bs := make([]byte, 4)
	binary.LittleEndian.PutUint32(bs, uint32(state))
	//msg := fmt.Sprintf("%v", test.state)
	n, err := test.ctrl_conn.Write(bs)
	if err != nil {
		log.Errorf("Write state error. %v %v", n, err)
		return -1
	}
	log.Debugf("Set & send state = %v, n = %v", state, n)
	return 0
}

func (test *iperf_test) new_stream(conn net.Conn, sender_flag int) *iperf_stream{
	sp := new(iperf_stream)
	sp.role = sender_flag
	sp.conn = conn
	sp.test = test

	// mark, set sp.buffer
	sp.result = new(iperf_stream_results)
	sp.snd = test.proto.send
	sp.rcv = test.proto.recv
	sp.buffer = make([]byte, test.setting.blksize)
	copy(sp.buffer[:], "hello world!")
	// initialize stream
	// set tos bit. undo
	return sp
}

func (test *iperf_test) close_all_streams() int{
	var err error
	for _, sp := range test.streams{
		err = sp.conn.Close()
		if err != nil {
			log.Errorf("Stream close failed, err = %v", err)
			return -1
		}
	}
	return 0
}

func (test *iperf_test) check_throttle(sp *iperf_stream, now time.Time) {
	if sp.test.done {
		return
	}
	dur := now.Sub(sp.result.start_time)
	sec := dur.Seconds()
	bits_per_second := float64(sp.result.bytes_sent * 8) / sec
	if bits_per_second < float64(sp.test.setting.rate) && sp.can_send == false{
		sp.can_send = true
		log.Debugf("sp.can_send turn TRUE. bits_per_second = %6.2f MB/s Required = %6.2f MB/s",
			bits_per_second/MB_TO_B/8, float64(sp.test.setting.rate) / MB_TO_B / 8)
	} else if bits_per_second > float64(sp.test.setting.rate) && sp.can_send == true{
		sp.can_send = false
		log.Debugf("sp.can_send turn FALSE. bits_per_second = %6.2f MB/s Required = %6.2f MB/s",
			bits_per_second/MB_TO_B/8, float64(sp.test.setting.rate) / MB_TO_B / 8)
	}
}

func (test *iperf_test) send_params() int {
	log.Debugf("Enter send_params")
	params := stream_params{
		ProtoName:	test.proto.name(),
		Duration:	test.duration,
		NoDelay:	test.no_delay,
		Interval:	test.interval,
		StreamNum:	test.stream_num,
		Blksize:	test.setting.blksize,
	}
	//encoder := json.NewEncoder(test.ctrl_conn)
	//err := encoder.Encode(params)

	bytes, err := json.Marshal(&params)
	if err != nil {
		log.Error("Encode params failed. %v", err)
		return -1
	}
	n, err := test.ctrl_conn.Write(bytes)
	if err != nil{
		log.Error("Write failed. %v", err)
		return -1
	}
	log.Debugf("send params %v bytes: %v", n, params.String())
	return 0
}

func (test *iperf_test) get_params() int {
	log.Debugf("Enter get_params")
	var params stream_params
	//encoder := json.NewDecoder(test.ctrl_conn)
	//err := encoder.Decode(&params)
	buf := make([]byte, 1024)
	n, err := test.ctrl_conn.Read(buf)
	if err != nil {
		log.Errorf("Read failed. %v", err)
		return -1
	}
	err = json.Unmarshal(buf[:n], &params)
	if err != nil {
		log.Errorf("Decode failed. %v", err)
		return -1
	}
	log.Debugf("get params %v bytes: %v", n, params.String())
	test.set_protocol(params.ProtoName)
	test.duration = params.Duration
	test.no_delay = params.NoDelay
	test.interval = params.Interval
	test.stream_num = params.StreamNum
	test.setting.blksize = params.Blksize
	return 0
}

func (test *iperf_test) exchange_params() int {
	if test.is_server == false {
		if test.send_params() < 0 {
			return -1
		}
	} else {
		if test.get_params() < 0 {
			return -1
		}
	}
	return 0
}

func (test *iperf_test) send_results() int {
	log.Debugf("Send Results")
	var results = make(stream_results_array, test.stream_num)
	for i, sp := range test.streams{
		var bytes_transfer uint64
		if test.is_server {
			bytes_transfer = sp.result.bytes_received
		} else {
			bytes_transfer = sp.result.bytes_sent
		}
		sp_result := stream_results_exchange{
			Id:			uint(i),
			Bytes:		bytes_transfer,
			Retrans: 	0,		// current not used
			Jitter:		0, 		// current not used
			Packets:	0,		// current not used
			StartTime: sp.result.start_time,
			EndTime:	sp.result.end_time,
		}
		results[i] = sp_result
	}
	bytes, err := json.Marshal(&results)
	if err != nil {
		log.Error("Encode results failed. %v", err)
		return -1
	}
	n, err := test.ctrl_conn.Write(bytes)
	if err != nil{
		log.Error("Write failed. %v", err)
		return -1
	}
	if test.is_server {
		log.Debugf("Server send results %v bytes: %v", n, results)
	} else {
		log.Debugf("Client send results %v bytes: %v", n, results)
	}

	return 0
}

func (test *iperf_test) get_results() int {
	log.Debugf("Enter get_results")
	var results = make(stream_results_array, test.stream_num)
	//encoder := json.NewDecoder(test.ctrl_conn)
	//err := encoder.Decode(&params)
	buf := make([]byte, 4*1024)
	n, err := test.ctrl_conn.Read(buf)
	if err != nil {
		log.Errorf("Read failed. %v", err)
		return -1
	}
	err = json.Unmarshal(buf[:n], &results)
	if err != nil {
		log.Errorf("Decode failed. %v", err)
		return -1
	}
	if test.is_server {
		log.Debugf("Server get results %v bytes: %v", n, results)
	} else {
		log.Debugf("Client get results %v bytes: %v", n, results)
	}


	for i, result := range results{
		sp := test.streams[i]
		if test.is_server {
			sp.result.bytes_sent = result.Bytes
			sp.result.stream_retrans = result.Retrans
		} else {
			sp.result.bytes_received = result.Bytes
			//sp.jitter = result.jitter
			//sp. = result.
		}
	}
	return 0
}

func (test *iperf_test) exchange_results() int {
	if test.is_server == false {
		if test.send_results() < 0 {
			return -1
		}
		if test.get_results() < 0 {
			return -1
		}
	} else {
		// server
		if test.get_results() < 0 {
			return -1
		}
		if test.send_results() < 0 {
			return -1
		}
	}
	return 0
}

func (test *iperf_test) init_test() int{
	test.proto.init(test)
	now := time.Now()
	for _, sp := range test.streams{
		sp.result.start_time = now
		sp.result.start_time_fixed = now
	}
	return 0
}

/*
	main level interface
 */
func (test *iperf_test) init() {
	test.protocols = append(test.protocols, new(tcp_proto), new(rudp_proto))
}

func (test *iperf_test)parse_arguments() int {

	// command flag definition
	var help_flag = flag.Bool("h", false, "this help")
	var server_flag = flag.Bool("s", false, "server side")
	var client_flag = flag.String("c", "127.0.0.1", "client side")
	var port_flag = flag.Uint("p", 5201, "connect/listen port")
	var protocol_flag = flag.String("proto", TCP_NAME, "protocol under test")
	var interval_flag = flag.Uint("i", 1, "test interval (ms)")
	var blksize_flag = flag.Uint("l", 1400, "send/read block size")
	var bandwidth_flag = flag.Uint("b", 0, "bandwidth limit. (Mb/s)")
	// parse argument
	flag.Parse()

	if *help_flag {
		flag.Usage()
	}
	// check valid
	if  flag.CommandLine.Lookup("c") == nil {
		if *server_flag == false{
			return -1
		}
	}
	valid_protocol := false
	var PROTOCOL_LIST = [3]string{TCP_NAME, UDP_NAME, RUDP_NAME}
	for _, proto := range PROTOCOL_LIST{
		if *protocol_flag == proto {
			valid_protocol = true
		}
	}
	if valid_protocol == false{
		return -2
	}

	// set block size
	if  flag.CommandLine.Lookup("l") == nil {
		if *protocol_flag == TCP_NAME {
			test.setting.blksize = DEFAULT_TCP_BLKSIZE
		} else if *protocol_flag == UDP_NAME {
			test.setting.blksize = DEFAULT_UDP_BLKSIZE
		} else if *protocol_flag == RUDP_NAME {
			test.setting.blksize = DEFAULT_RUDP_BLKSIZE
		}
	} else {
		test.setting.blksize = *blksize_flag
	}

	if  flag.CommandLine.Lookup("b") == nil {
		test.setting.burst = true
	} else {
		test.setting.burst = false
		test.setting.rate = *bandwidth_flag * MB_TO_B * 8
		test.setting.pacing_time = 5		// 5ms pacing
	}
	// pass to iperf_test
	if *server_flag == true{
		test.is_server = true
	} else{
		test.is_server = false
		var err error
		_, err = net.ResolveIPAddr("ip", *client_flag)
		if err != nil {
			return -3
		}
		test.addr = *client_flag
	}

	test.port = *port_flag
	test.state = 0
	test.interval = *interval_flag
	test.duration = 10000	// 10s
	test.no_delay = true
	if test.is_server == false {
		test.set_protocol(*protocol_flag)
	}

	test.Print()
	return 0
}

func (test *iperf_test) run_test() int {
	// server
	if test.is_server == true {
		rtn := test.run_server()
		if rtn < 0 {
			log.Errorf("Run server failed. %v", rtn)
			return rtn
		}

	} else {
	//client
		rtn := test.run_client()
		if rtn < 0 {
			log.Errorf("Run client failed. %v", rtn)
			return rtn
		}
	}

	return 0
}

func (test *iperf_test) free_test() int {
	return 0
}

func (test *iperf_test) Print() {
	var proto_name string
	if test.proto != nil {
		proto_name = test.proto.name()
	}

	fmt.Printf("Iperf test detail:\n")
	fmt.Printf("IsServer:%v\taddr:%v\tport:%v\tstate:%v\tproto:%v\tno_delay:%v\tinterval:%v\tduration:%v\t",
		test.is_server, test.addr, test.port, test.state, proto_name, test.no_delay, test.interval, test.duration)

}



/*
	----------------------------------------------------
	******************* iperf_stream *******************
	----------------------------------------------------
 */
func (sp *iperf_stream) iperf_recv(test *iperf_test) {
	// travel all the stream and start receive
	for {
		var n int
		if n = sp.rcv(sp); n < 0{
			if n == -1 {
				log.Debugf("Server Quit receiving")
				return
			}
			log.Errorf("Iperf streams receive failed. n = %v", n)
			return
		}
		test.bytes_received += uint64(n)
		test.blocks_received += 1
		//log.Debugf("Stream receive data %v bytes of total %v bytes", n, test.bytes_received)
	}
}

/*
	called by multi streams. Be careful the function called here
 */
func (sp *iperf_stream) iperf_send(test *iperf_test) {
	// travel all the stream and start receive
	for{
		if sp.can_send {
			var n int
			if n = sp.snd(sp); n < 0{
				if n == -1 {
					log.Debugf("Iperf send stream closed.")
					return
				}
				log.Error("Iperf streams send failed. %v", n)
				return
			}
			test.bytes_sent += uint64(n)
			test.blocks_sent += 1
			//log.Debugf("Stream sent data %v bytes of total %v bytes", n, test.bytes_sent)
		}
		if test.setting.burst == false {
			test.check_throttle(sp, time.Now())
		}
		if (test.duration != 0 && test.done) ||
			(test.setting.bytes != 0 && test.bytes_sent >= test.setting.bytes) ||
			(test.setting.blocks != 0 && test.blocks_sent >= test.setting.blocks){
			test.ctrl_chan <- TEST_END
			// end sending
			log.Debugf("Client Quit sending")
			return
		}
	}
}
/*
	----------------------------------------------------
	****************** call_back func ******************
	----------------------------------------------------
 */

// Main report-printing callback.
func iperf_reporter_callback(test *iperf_test){
	<- test.chStats		// only call this function after stats
	if test.state == TEST_RUNNING {
		log.Debugf("TEST_RUNNING report, role = %v, done = %v", test.is_server, test.done)
		test.iperf_print_intermediate()
	} else if test.state == TEST_END || test.state == IPERF_DISPLAY_RESULT {
		log.Debugf("TEST_END report, role = %v, done = %v", test.is_server, test.done)
		test.iperf_print_intermediate()
		test.iperf_print_results()
	} else {
		log.Errorf("Unexpected state = %v, role = %v", test.state, test.is_server)
	}
}

func (test *iperf_test)iperf_print_intermediate(){
	var sum_bytes_transfer, sum_rtt uint64
	var display_start_time, display_end_time float64
	for i, sp := range test.streams{
		if i == 0 && len(sp.result.interval_results) == 1{
			// first time to print result, print header
			fmt.Printf(TCP_REPORT_HEADER)
		}
		interval_seq := len(sp.result.interval_results) - 1
		rp := sp.result.interval_results[interval_seq]		// get the last one
		supposed_start_time := time.Duration(uint(interval_seq) * test.interval) * time.Millisecond
		real_start_time := rp.interval_start_time.Sub(sp.result.start_time)
		real_end_time := rp.interval_end_time.Sub(sp.result.start_time)
		if dur_not_same(supposed_start_time, real_start_time) {
			log.Errorf("Start time differ from expected. supposed = %v, real = %v",
				supposed_start_time.Nanoseconds() / MS_TO_NS, real_start_time.Nanoseconds() / MS_TO_NS)
			return
		}
		sum_bytes_transfer += rp.bytes_transfered
		sum_rtt += uint64(rp.rtt)
		display_start_time = float64(real_start_time.Nanoseconds())/ S_TO_NS
		display_end_time = float64(real_end_time.Nanoseconds())/ S_TO_NS
		display_bytes_transfer := float64(rp.bytes_transfered) / MB_TO_B
		display_bandwidth := display_bytes_transfer / float64(test.interval) * 1000 * 8	// Mb/s
		// output single stream interval report
		fmt.Printf(TCP_REPORT_SINGLE_STREAM, i,
			display_start_time, display_end_time, display_bytes_transfer, display_bandwidth, float64(rp.rtt)/1000)
	}
	if test.stream_num > 1 {
		display_sum_bytes_transfer := float64(sum_bytes_transfer) / MB_TO_B
		display_bandwidth := display_sum_bytes_transfer /  float64(test.interval) * 1000 * 8
		fmt.Printf(TCP_REPORT_SUM_STREAM, display_start_time, display_end_time, display_sum_bytes_transfer,
			display_bandwidth, float64(sum_rtt)/1000/float64(test.stream_num))
		fmt.Printf(REPORT_SEPERATOR)
	}
}

func dur_not_same(d time.Duration, d2 time.Duration) bool {
	// if deviation exceed 1ms, there might be problems
	var diff_in_ms int = int(d.Nanoseconds() / MS_TO_NS - d2.Nanoseconds() / MS_TO_NS)
	if diff_in_ms < -10 || diff_in_ms > 10 {
		return true
	}
	return false
}

func (test *iperf_test)iperf_print_results(){
	fmt.Printf(SUMMARY_SEPERATOR)
	fmt.Printf(TCP_REPORT_HEADER)
	if len(test.streams) <=0 {
		log.Errorf("No streams available.")
		return
	}
	var sum_bytes_transfer uint64
	var avg_rtt float64
	var display_start_time, display_end_time float64
	for i, sp := range test.streams{
		display_start_time = float64(0)
		display_end_time = float64(sp.result.end_time.Sub(sp.result.start_time).Nanoseconds())/ S_TO_NS
		var display_bytes_transfer float64
		if test.is_server {
			display_bytes_transfer = float64(sp.result.bytes_received) / MB_TO_B
			sum_bytes_transfer += sp.result.bytes_received
		} else {
			display_bytes_transfer = float64(sp.result.bytes_sent) / MB_TO_B
			sum_bytes_transfer += sp.result.bytes_sent
		}
		display_rtt := float64(sp.result.stream_sum_rtt) / float64(sp.result.stream_cnt_rtt)
		avg_rtt += display_rtt
		display_bandwidth := display_bytes_transfer / float64(test.duration) * 8		// Mb/s
		// output single stream final report
		fmt.Printf(TCP_REPORT_SINGLE_STREAM, i, display_start_time,
			display_end_time, display_bytes_transfer, display_bandwidth, display_rtt)
	}
	if test.stream_num > 1 {
		display_sum_bytes_transfer := float64(sum_bytes_transfer) / MB_TO_B
		display_bandwidth := display_sum_bytes_transfer /  float64(test.duration) * 1000 * 8
		fmt.Printf(TCP_REPORT_SUM_STREAM, display_start_time, display_end_time,
			display_sum_bytes_transfer, display_bandwidth, avg_rtt / float64(test.stream_num))
	}
}

// Gather statistics during a test.
func iperf_stats_callback(test *iperf_test){
	for _, sp := range test.streams{
		temp_result := iperf_interval_results{}
		rp := sp.result
		if len(rp.interval_results) == 0 {
			// first interval
			temp_result.interval_start_time = rp.start_time
		} else {
			temp_result.interval_start_time = rp.end_time	// rp.end_time contains timestamp of previous interval
		}
		rp.end_time = time.Now()
		temp_result.interval_end_time = rp.end_time
		temp_result.interval_dur = temp_result.interval_end_time.Sub(temp_result.interval_start_time)
		test.proto.stats_callback(test, sp, &temp_result)	// write temp_result differ from proto to proto
		if test.is_server {
			temp_result.bytes_transfered = rp.bytes_received_this_interval
		} else {
			temp_result.bytes_transfered = rp.bytes_sent_this_interval
		}
		rp.interval_results = append(rp.interval_results, temp_result)
		rp.bytes_sent_this_interval = 0
		rp.bytes_received_this_interval = 0
	}
	test.chStats <- true
}