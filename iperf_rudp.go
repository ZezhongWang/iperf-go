package main

import (
	"github.com/pkg/errors"
	"net"
)

type rudp_proto struct{
}

func (rudp *rudp_proto) name() string{
	return RUDP_NAME
}

func (rudp *rudp_proto) accept(test *iperf_test) (net.Conn, error){
	return nil, errors.New("Accept Unfinished.")
}

func (rudp *rudp_proto) listen(test *iperf_test) (net.Listener, error){
	return nil, errors.New("Listen Unfinished.")
}

func (rudp *rudp_proto) connect(test *iperf_test) (net.Conn, error){
	return nil, errors.New("Connect Unfinished.")
}

func (rudp *rudp_proto) send(test *iperf_stream) int{
	return 0
}

func (rudp *rudp_proto) recv(test *iperf_stream) int{
	return 0
}

func (rudp *rudp_proto) init(test *iperf_test) int{
	return 0
}

func (rudp *rudp_proto) stats_callback(test *iperf_test, sp *iperf_stream, temp_result *iperf_interval_results) int {
	return 0
}