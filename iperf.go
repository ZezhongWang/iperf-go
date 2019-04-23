package main

import (
	"fmt"
	"net"
	"time"
)

var PROTOCOL_LIST = [3]string{"tcp", "udp", "rudp"}

const(
	IPERF_START   			= 1
	IPERF_DONE	  			= 2
	IPERF_CREATE_STREAM  	= 3
	IPERF_EXCHANGE_PARAMS	= 4
	IPERF_EXCHANGE_RESULT	= 5
	IPERF_DISPLAY_RESULT	= 6

	STREAM_CLOSE			= 10

	SENDER_STREAM			= 20
	RECEIVER_STREAM			= 21

	TEST_START				= 31
	TEST_RUNNING			= 32
	TEST_RESULT_REQUEST		= 33
	TEST_END				= 34
	/* unexpected situation */
	CLIENT_TERMINATE		= 50
	SERVER_TERMINATE		= 51
)

const(
	TCP_NAME	= "tcp"
	UDP_NAME	= "udp"
	RUDP_NAME	= "rudp"
)

const(
	DEFAULT_TCP_BLKSIZE		= 128*1024	// default read/write block size
	DEFAULT_UDP_BLKSIZE 	= 1460		// default is dynamically set
	DEFAULT_RUDP_BLKSIZE		= 128*1024	// default read/write block size

	MS_TO_NS 				= 1000000
	S_TO_NS					= 1000000000
	MB_TO_B					= 1024*1024
)

const(
	TCP_REPORT_HEADER 	= "[ ID]    Interval        Transfer        Bandwidth        RTT\n"
	TCP_REPORT_SINGLE_STREAM = "[  %v] %4.2f-%4.2f sec\t%5.2f MB\t%5.2f Mb/s\t%6.1fms\n"
	TCP_REPORT_SUM_STREAM 	 = "[SUM] %4.2f-%4.2f sec\t%5.2f MB\t%5.2f Mb/s\t%6.1fms\n"
	REPORT_SEPERATOR 	= "- - - - - - - - - - - - - - - - - - - - - - - - - - - -\n"
	SUMMARY_SEPERATOR 	= "- - - - - - - - - - - - SUMMARY - - - - - - - - - - - -\n"
)
type iperf_test struct {
	is_server	bool
	addr		string
	port		uint
	state 		uint
	duration 	uint	// sec
	no_delay	bool
	interval	uint	// ms
	proto 		protocol
	protocols	[]protocol


	/* stream */

	listener		net.Listener
	proto_listener	net.Listener
	ctrl_conn		net.Conn
	ctrl_chan		chan uint
	setting 		*iperf_setting
	stream_num		uint
	streams			[]*iperf_stream

	/* test statistics */
	bytes_received	uint64
	blocks_received	uint64
	bytes_sent		uint64
	blocks_sent		uint64
	done 			bool

	/* timer */
	timer 			ITimer
	//omit_timer 		ITimer  // not used yet
	stats_ticker	ITicker
	report_ticker 	ITicker
	chStats			chan bool

	/* call back function */

	stats_callback func(test *iperf_test)
	reporter_callback func(test *iperf_test)
	//on_new_stream 	on_new_stream_callback
	//on_test_start 	on_test_start_callback
	//on_connect 		on_connect_callback
	//on_test_finish 	on_test_finish_callback
}

// output_callback is a prototype which ought capture conn and call conn.Write
//type on_new_stream_callback func(test *iperf_stream)
//type on_test_start_callback func(test *iperf_test)
//type on_connect_callback func(test *iperf_test)
//type on_test_finish_callback func(test *iperf_test)


type protocol interface {
	//name string
	name()	string
	accept(test *iperf_test) (net.Conn, error)
	listen(test *iperf_test) (net.Listener, error)
	connect(test *iperf_test) (net.Conn, error)
	send(test *iperf_stream) int
	recv(test *iperf_stream) int
	init(test *iperf_test) int
	stats_callback(test *iperf_test, sp *iperf_stream, temp_result *iperf_interval_results) int
}

type iperf_stream struct{
	role            int 	//SENDER_STREAM or RECEIVE_STREAM
	test            *iperf_test
	result			*iperf_stream_results
	can_send		bool
	conn			net.Conn
	send_ticker		ITicker

	buffer  		[]byte	//buffer to send

	rcv func(sp *iperf_stream) int		// return recv size. -1 represent EOF.
	snd func(sp *iperf_stream) int		// return send size. -1 represent socket close.

}

type iperf_setting struct{
	skt_bufsize 	uint
	blksize			uint
	burst			bool		// burst & rate & pacing_time should be set at the same time
	rate			uint		// bit per second
	pacing_time 	uint		// ms
	bytes 			uint64
	blocks 			uint64
}

// params to exchange
// tips: all the members should be visible, or json decoder cannot encode it
type stream_params struct{
	ProtoName		string
	Duration		uint
	NoDelay			bool
	Interval		uint
	StreamNum		uint
	Blksize			uint
}

func (p stream_params) String() string{
	s := fmt.Sprintf("name:%v\tdur:%v\tno_delay:%v\tinterval:%v\tstream_num:%v\t",
		p.ProtoName, p.Duration, p.NoDelay, p.Interval, p.StreamNum)
	return s
}

type iperf_stream_results struct{
	bytes_received					uint64
	bytes_sent						uint64
	bytes_received_this_interval	uint64
	bytes_sent_this_interval		uint64
	bytes_sent_omit					uint64
	stream_retrans 					uint
	stream_prev_total_retrans		uint
	stream_max_rtt					uint
	stream_min_rtt					uint
	stream_sum_rtt					uint		// micro sec
	stream_cnt_rtt					uint
	start_time						time.Time
	end_time						time.Time
	start_time_fixed				time.Time
	interval_results                []iperf_interval_results
}

type stream_results_array []stream_results_exchange

// result to exchange
// tips: all the members should be visible, or json decoder cannot encode it
type stream_results_exchange struct{
	Id 			uint
	Bytes 		uint64
	Retrans		uint
	Jitter 		uint
	Packets 	uint
	StartTime	time.Time
	EndTime	time.Time
}

func (r stream_results_exchange) String() string{
	s := fmt.Sprintf("id:%v\tbytes:%v\tretrans:%v\tjitter:%v\tpackets:%v\tstart_time:%v\tend_time:%v\t",
		r.Id, r.Bytes, r.Retrans, r.Jitter, r.Packets, r.StartTime, r.EndTime)
	return s
}

type iperf_interval_results struct{
	bytes_transfered				uint64
	interval_start_time				time.Time
	interval_end_time				time.Time
	interval_dur					time.Duration
	rtt 							uint		// micro sec !
	interval_retrans				uint		// bytes
	/* for udp */
	interval_packet_cnt				uint
	omitted							uint
}