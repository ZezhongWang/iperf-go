package main

import (
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/op/go-logging"
	"net"
	"os"
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
		Reverse:	test.reverse,
		Duration:	test.duration,
		NoDelay:	test.no_delay,
		Interval:	test.interval,
		StreamNum:	test.stream_num,
		Blksize:	test.setting.blksize,
		SndWnd:		test.setting.snd_wnd,
		RcvWnd:		test.setting.rcv_wnd,
		ReadBufSize: test.setting.read_buf_size,
		WriteBufSize: test.setting.write_buf_size,
		FlushInterval: test.setting.flush_interval,
		NoCong:		test.setting.no_cong,
		FastResend: test.setting.fast_resend,
		DataShards: test.setting.data_shards,
		ParityShards: test.setting.parity_shards,
		Burst:		test.setting.burst,
		Rate:		test.setting.rate,
		PacingTime: test.setting.pacing_time,
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
	test.set_test_reverse(params.Reverse)
	test.duration = params.Duration
	test.no_delay = params.NoDelay
	test.interval = params.Interval
	test.stream_num = params.StreamNum
	test.setting.blksize = params.Blksize
	test.setting.burst = params.Burst
	test.setting.rate = params.Rate
	test.setting.pacing_time = params.PacingTime
	// rudp/kcp only
	test.setting.snd_wnd = params.SndWnd
	test.setting.rcv_wnd = params.RcvWnd
	test.setting.write_buf_size = params.WriteBufSize
	test.setting.read_buf_size = params.ReadBufSize
	test.setting.flush_interval = params.FlushInterval
	test.setting.no_cong = params.NoCong
	test.setting.fast_resend = params.FastResend
	test.setting.data_shards = params.DataShards
	test.setting.parity_shards = params.ParityShards
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
		if test.mode == IPERF_RECEIVER {
			bytes_transfer = sp.result.bytes_received
		} else {
			bytes_transfer = sp.result.bytes_sent
		}
		rp := sp.result
		sp_result := stream_results_exchange{
			Id:			uint(i),
			Bytes:		bytes_transfer,
			Retrans: 	rp.stream_retrans,
			Jitter:		0, 		// current not used
			InPkts:		rp.stream_in_pkts,
			OutPkts: 	rp.stream_out_pkts,
			InSegs:		rp.stream_in_segs,
			OutSegs:	rp.stream_out_segs,
			Recovered:  rp.stream_recovers,
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
		if test.mode == IPERF_RECEIVER {
			sp.result.bytes_sent = result.Bytes
			sp.result.stream_retrans = result.Retrans
			sp.result.stream_out_segs = result.OutSegs
			sp.result.stream_out_pkts = result.OutPkts
		} else {
			sp.result.bytes_received = result.Bytes
			//sp.jitter = result.jitter
			sp.result.stream_in_segs = result.InSegs
			sp.result.stream_in_pkts = result.InPkts
			sp.result.stream_recovers = result.Recovered
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
	test.protocols = append(test.protocols, new(tcp_proto), new(rudp_proto), new(kcp_proto))
}

func (test *iperf_test)parse_arguments() int {

	// command flag definition
	var help_flag = flag.Bool("h", false, "this help")
	var server_flag = flag.Bool("s", false, "server side")
	var client_flag = flag.String("c", "127.0.0.1", "client side")
	var reverse_flag = flag.Bool("R", false, "reverse mode. client receive, server send")
	var port_flag = flag.Uint("p", 5201, "connect/listen port")
	var protocol_flag = flag.String("proto", TCP_NAME, "protocol under test")
	var dur_flag = flag.Uint("d", 10, "duration (s)")
	var interval_flag = flag.Uint("i", 1000, "test interval (ms)")
	var parallel_flag = flag.Uint("P", 1, "The number of simultaneous connections")
	var blksize_flag = flag.Uint("l", 4*1024, "send/read block size")
	var bandwidth_flag = flag.Uint("b", 0, "bandwidth limit. (Mb/s)")
	var debug_flag = flag.Bool("debug", false, "debug mode")
	var info_flag = flag.Bool("info", false, "info mode")
	var no_delay_flag = flag.Bool("D", false, "no delay option")
	// RUDP specific option
	var snd_wnd_flag = flag.Uint("sw", 10, "rudp send window size")
	var rcv_wnd_flag = flag.Uint("rw", 512, "rudp receive window size")
	var read_buffer_size_flag = flag.Uint("rb", 4*1024, "read buffer size (Kb)")
	var write_buffer_size_flag = flag.Uint("wb", 4*1024, "write buffer size (Kb)")
	var flush_interval_flag = flag.Uint("f", 10, "flush interval for rudp (ms)")
	var no_cong_flag = flag.Bool("nc", true, "no congestion control or BBR")
	var fast_resend_flag = flag.Uint("fr", 0, "rudp fast resend strategy. 0 indicate turn off fast resend")
	var dataShards_flag = flag.Uint("data", 0, "rudp/kcp FEC dataShards option")
	var parityShards_flag = flag.Uint("parity", 0, "rudp/kcp FEC parityShards option")
	// parse argument
	flag.Parse()

	if *help_flag {
		flag.Usage()
		os.Exit(0)
	}
	// check valid
	flagset := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) { flagset[f.Name]=true } )

	if  flagset["c"] == false{
		if *server_flag == false{
			return -1
		}
	}
	valid_protocol := false
	for _, proto := range PROTOCOL_LIST{
		if *protocol_flag == proto {
			valid_protocol = true
		}
	}
	if valid_protocol == false{
		return -2
	}
	//if flagset["nc"] == true{
	//	test.setting.no_cong = true
	//} else {
	//	test.setting.no_cong = false
	//}

	// set block size
	if  flagset["l"] == false{
		if *protocol_flag == TCP_NAME {
			test.setting.blksize = DEFAULT_TCP_BLKSIZE
		} else if *protocol_flag == UDP_NAME {
			test.setting.blksize = DEFAULT_UDP_BLKSIZE
		} else if *protocol_flag == RUDP_NAME {
			test.setting.blksize = DEFAULT_RUDP_BLKSIZE
		} else if *protocol_flag == KCP_NAME {
			test.setting.blksize = DEFAULT_RUDP_BLKSIZE
		}
	} else {
		test.setting.blksize = *blksize_flag
	}

	if  flagset["b"] == false{
		test.setting.burst = true
	} else {
		test.setting.burst = false
		test.setting.rate = *bandwidth_flag * MB_TO_B * 8
		test.setting.pacing_time = 5		// 5ms pacing
	}

	if *debug_flag == true{
		logging.SetLevel(logging.DEBUG, "iperf")
		logging.SetLevel(logging.DEBUG, "rudp")
	} else if *info_flag == true{
		logging.SetLevel(logging.INFO, "iperf")
		logging.SetLevel(logging.INFO, "rudp")
	} else {
		logging.SetLevel(logging.ERROR, "iperf")
		logging.SetLevel(logging.ERROR, "rudp")
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
	test.set_test_reverse(*reverse_flag)
	test.port = *port_flag
	test.state = 0
	test.interval = *interval_flag
	test.duration = *dur_flag		// 10s
	test.stream_num = *parallel_flag
	// rudp only
	test.setting.snd_wnd = *snd_wnd_flag
	test.setting.rcv_wnd = *rcv_wnd_flag
	test.setting.read_buf_size = *read_buffer_size_flag * 1024	// Kb to b
	test.setting.write_buf_size = *write_buffer_size_flag * 1024
	test.setting.flush_interval = *flush_interval_flag
	test.setting.no_cong = *no_cong_flag
	test.setting.fast_resend = *fast_resend_flag
	test.setting.data_shards = *dataShards_flag
	test.setting.parity_shards = *parityShards_flag

	if test.interval > test.duration * 1000{
		log.Errorf("interval must smaller than duration")
	}
	test.no_delay = *no_delay_flag
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

func (test *iperf_test) set_test_reverse(reverse bool) {
	test.reverse = reverse
	if reverse == true{
		if test.is_server {
			test.mode = IPERF_SENDER
		} else {
			test.mode = IPERF_RECEIVER
		}
	} else {
		if test.is_server {
			test.mode = IPERF_RECEIVER
		} else {
			test.mode = IPERF_SENDER
		}
	}
}

func (test *iperf_test) free_test() int {
	return 0
}

func (test *iperf_test) Print() {
	if test.is_server {
		return
	}
	if test.proto == nil {
		log.Errorf("Protocol not set.")
		return
	}
	fmt.Printf("Iperf started:\n")
	if test.proto.name() == TCP_NAME{
		fmt.Printf("addr:%v\tport:%v\tproto:%v\tinterval:%v\tduration:%v\tNoDelay:%v\tburst:%v\tBlockSize:%v\tStreamNum:%v\n",
			test.addr, test.port, test.proto.name(), test.interval, test.duration, test.no_delay, test.setting.burst, test.setting.blksize, test.stream_num)
	} else if test.proto.name() == RUDP_NAME{
		fmt.Printf("addr:%v\tport:%v\tproto:%v\tinterval:%v\tduration:%v\tNoDelay:%v\tburst:%v\tBlockSize:%v\tStreamNum:%v\n" +
			"RUDP settting: sndWnd:%v\trcvWnd:%v\twriteBufSize:%vKb\treadBufSize:%vKb\tnoCongestion:%v\tflushInterval:%v\tdataShards:%v\tparityShards:%v\n",
			test.addr, test.port, test.proto.name(), test.interval, test.duration, test.no_delay, test.setting.burst, test.setting.blksize, test.stream_num,
			test.setting.snd_wnd, test.setting.rcv_wnd, test.setting.write_buf_size / 1024, test.setting.read_buf_size / 1024, test.setting.no_cong,
			test.setting.flush_interval, test.setting.data_shards, test.setting.parity_shards)
	} else if test.proto.name() == KCP_NAME{
		fmt.Printf("addr:%v\tport:%v\tproto:%v\tinterval:%v\tduration:%v\tNoDelay:%v\tburst:%v\tBlockSize:%v\tStreamNum:%v\n" +
			"KCP settting: sndWnd:%v\trcvWnd:%v\twriteBufSize:%vKb\treadBufSize:%vKb\tnoCongestion:%v\tflushInterval:%v\tdataShards:%v\tparityShards:%v\n",
			test.addr, test.port, test.proto.name(), test.interval, test.duration, test.no_delay, test.setting.burst, test.setting.blksize, test.stream_num,
			test.setting.snd_wnd, test.setting.rcv_wnd, test.setting.write_buf_size / 1024, test.setting.read_buf_size / 1024, test.setting.no_cong,
			test.setting.flush_interval, test.setting.data_shards, test.setting.parity_shards)
	}
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
				log.Debugf("Stream Quit receiving")
				return
			}
			log.Errorf("Iperf streams receive failed. n = %v", n)
			return
		}
		if test.state == TEST_RUNNING {
			test.bytes_received += uint64(n)
			test.blocks_received += 1
			log.Debugf("Stream receive data %v bytes of total %v bytes", n, test.bytes_received)
		}
		if test.done {
			test.ctrl_chan <- TEST_END
			log.Debugf("Stream quit receiving. test done.")
			return
		}
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
			log.Debugf("Stream sent data %v bytes of total %v bytes", n, test.bytes_sent)
		}
		if test.setting.burst == false {
			test.check_throttle(sp, time.Now())
		}
		if (test.duration != 0 && test.done) ||
			(test.setting.bytes != 0 && test.bytes_sent >= test.setting.bytes) ||
			(test.setting.blocks != 0 && test.blocks_sent >= test.setting.blocks){
			test.ctrl_chan <- TEST_END
			// end sending
			log.Debugf("Stream Quit sending")
			return
		}
	}
}

func (test *iperf_test) create_sender_ticker() int {
	for _, sp := range test.streams{
		sp.can_send = true
		if test.setting.rate != 0 {
			if test.setting.pacing_time == 0 || test.setting.burst == true {
				log.Error("pacing_time & rate & burst should be set at the same time.")
				return -1
			}
			var cd TimerClientData
			cd.p = sp
			sp.send_ticker = ticker_create(time.Now(), send_ticker_proc, cd, test.setting.pacing_time, ^uint(0))
		}
	}
	return 0
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
		log.Debugf("TEST_RUNNING report, role = %v, mode = %v, done = %v", test.is_server, test.mode, test.done)
		test.iperf_print_intermediate()
	} else if test.state == TEST_END || test.state == IPERF_DISPLAY_RESULT {
		log.Debugf("TEST_END report, role = %v, mode = %v, done = %v", test.is_server, test.mode, test.done)
		test.iperf_print_intermediate()
		test.iperf_print_results()
	} else {
		log.Errorf("Unexpected state = %v, role = %v", test.state, test.is_server)
	}
}

func (test *iperf_test)iperf_print_intermediate(){
	var sum_bytes_transfer, sum_rtt uint64
	var sum_retrans uint
	var display_start_time, display_end_time float64
	for i, sp := range test.streams{
		if i == 0 && len(sp.result.interval_results) == 1{
			// first time to print result, print header
			if test.proto.name() == TCP_NAME {
				fmt.Printf(TCP_INTERVAL_HEADER)
			} else {
				fmt.Printf(RUDP_INTERVAL_HEADER)
			}
		}
		interval_seq := len(sp.result.interval_results) - 1
		rp := sp.result.interval_results[interval_seq]		// get the last one
		supposed_start_time := time.Duration(uint(interval_seq) * test.interval) * time.Millisecond
		real_start_time := rp.interval_start_time.Sub(sp.result.start_time)
		real_end_time := rp.interval_end_time.Sub(sp.result.start_time)
		if dur_not_same(supposed_start_time, real_start_time) {
			log.Errorf("Start time differ from expected. supposed = %v, real = %v",
				supposed_start_time.Nanoseconds() / MS_TO_NS, real_start_time.Nanoseconds() / MS_TO_NS)
			//return
		}
		sum_bytes_transfer += rp.bytes_transfered
		sum_retrans += rp.interval_retrans
		sum_rtt += uint64(rp.rtt)
		display_start_time = float64(real_start_time.Nanoseconds())/ S_TO_NS
		display_end_time = float64(real_end_time.Nanoseconds())/ S_TO_NS
		display_bytes_transfer := float64(rp.bytes_transfered) / MB_TO_B
		display_bandwidth := display_bytes_transfer / float64(test.interval) * 1000 * 8	// Mb/s
		// output single stream interval report
		if test.proto.name() == TCP_NAME {
			//display_retrans_rate :=  float64(rp.interval_retrans) / (float64(rp.bytes_transfered) / TCP_MSS) * 100
			fmt.Printf(TCP_REPORT_SINGLE_STREAM, i, display_start_time, display_end_time,
				display_bytes_transfer, display_bandwidth, float64(rp.rtt)/1000, rp.interval_retrans)
		} else {
			total_segs := float64(rp.bytes_transfered) / RUDP_MSS + float64(rp.interval_retrans)
			display_retrans_rate := float64(rp.interval_retrans) / total_segs * 100		// to percentage
			display_lost_rate := float64(rp.interval_lost) / total_segs * 100
			display_early_retrans_rate := float64(rp.interval_early_retrans) / total_segs * 100
			display_fast_retrans_rate := float64(rp.interval_fast_retrans) / total_segs * 100
			fmt.Printf(RUDP_REPORT_SINGLE_STREAM, i, display_start_time, display_end_time, display_bytes_transfer,
				display_bandwidth, float64(rp.rtt)/1000, rp.interval_retrans, display_retrans_rate,
				display_lost_rate, display_early_retrans_rate, display_fast_retrans_rate)
		}
	}
	if test.stream_num > 1 {
		display_sum_bytes_transfer := float64(sum_bytes_transfer) / MB_TO_B
		display_bandwidth := display_sum_bytes_transfer /  float64(test.interval) * 1000 * 8
		fmt.Printf(REPORT_SUM_STREAM, display_start_time, display_end_time, display_sum_bytes_transfer,
			display_bandwidth, float64(sum_rtt)/1000/float64(test.stream_num), sum_retrans)
		fmt.Printf(REPORT_SEPERATOR)
	}
}

func dur_not_same(d time.Duration, d2 time.Duration) bool {
	// if deviation exceed 1ms, there might be problems
	var diff_in_ms int = int(d.Nanoseconds() / MS_TO_NS - d2.Nanoseconds() / MS_TO_NS)
	if diff_in_ms < -100 || diff_in_ms > 100 {
		return true
	}
	return false
}

func (test *iperf_test)iperf_print_results(){
	fmt.Printf(SUMMARY_SEPERATOR)
	if test.proto.name() == TCP_NAME {
		fmt.Printf(TCP_RESULT_HEADER)
	} else {
		fmt.Printf(RUDP_RESULT_HEADER)
	}
	
	if len(test.streams) <=0 {
		log.Errorf("No streams available.")
		return
	}
	var sum_bytes_transfer uint64
	var sum_retrans uint
	var avg_rtt float64
	var display_start_time, display_end_time float64
	for i, sp := range test.streams{
		display_start_time = float64(0)
		display_end_time = float64(sp.result.end_time.Sub(sp.result.start_time).Nanoseconds())/ S_TO_NS
		var display_bytes_transfer float64
		if test.mode == IPERF_RECEIVER {
			display_bytes_transfer = float64(sp.result.bytes_received) / MB_TO_B
			sum_bytes_transfer += sp.result.bytes_received
		} else {
			display_bytes_transfer = float64(sp.result.bytes_sent) / MB_TO_B
			sum_bytes_transfer += sp.result.bytes_sent
		}
		display_rtt := float64(sp.result.stream_sum_rtt) / float64(sp.result.stream_cnt_rtt) / 1000
		avg_rtt += display_rtt
		display_bandwidth := display_bytes_transfer / float64(test.duration) * 8		// Mb/s
		sum_retrans += sp.result.stream_retrans
		var role string
		if sp.role == SENDER_STREAM {
			role = "SENDER"
		} else {
			role = "RECEIVER"
		}
		// output single stream final report
		if test.proto.name() == TCP_NAME {
			total_segs := (display_bytes_transfer * MB_TO_B / TCP_MSS) + float64(sp.result.stream_retrans)
			display_retrans_rate := float64(sp.result.stream_retrans) / total_segs
			fmt.Printf(TCP_REPORT_SINGLE_RESULT, i, display_start_time, display_end_time, display_bytes_transfer,
				display_bandwidth, display_rtt, sp.result.stream_retrans, display_retrans_rate, role)
		} else {
			total_segs := float64(sp.result.stream_out_segs)
			display_retrans_rate := float64(sp.result.stream_retrans) / total_segs * 100
			display_lost_rate := float64(sp.result.stream_lost) / total_segs * 100
			display_early_retrans_rate := float64(sp.result.stream_early_retrans) / total_segs * 100
			display_fast_retrans_rate := float64(sp.result.stream_fast_retrans) / total_segs * 100

			recover_rate := float64(sp.result.stream_recovers) / total_segs * 100
			pkts_lost_rate := (1 - float64(sp.result.stream_in_pkts) / float64(sp.result.stream_out_pkts)) * 100
			segs_lost_rate := (1 - float64(sp.result.stream_in_segs) / float64(sp.result.stream_out_segs)) * 100
			fmt.Printf(RUDP_REPORT_SINGLE_RESULT, i, display_start_time, display_end_time, display_bytes_transfer,
				display_bandwidth, display_rtt, sp.result.stream_retrans, display_retrans_rate,
				display_lost_rate, display_early_retrans_rate, display_fast_retrans_rate,
				recover_rate, pkts_lost_rate, segs_lost_rate, role)
			fmt.Printf("total_segs = %v, out_segs = %v, in_segs = %v, out_pkts = %v, in_pkts = %v, recovery = %v\n",
				total_segs, sp.result.stream_out_segs, sp.result.stream_in_segs, sp.result.stream_out_pkts, sp.result.stream_in_pkts, sp.result.stream_recovers)
		}
	}
	if test.stream_num > 1 {
		display_sum_bytes_transfer := float64(sum_bytes_transfer) / MB_TO_B
		display_bandwidth := display_sum_bytes_transfer /  float64(test.duration) * 1000 * 8
		fmt.Printf(REPORT_SUM_STREAM, display_start_time, display_end_time,
			display_sum_bytes_transfer, display_bandwidth, avg_rtt / float64(test.stream_num), sum_retrans)
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
		if test.mode == IPERF_RECEIVER {
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