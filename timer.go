package main

import "time"

type ITimer struct{
	timer *time.Timer
	done chan bool
}

type ITicker struct{
	ticker *time.Ticker
	done chan bool
}

type TimerClientData struct{
	p interface{}
}
type timerProc func(data TimerClientData, now time.Time)


func timer_create(now time.Time, proc timerProc, data TimerClientData, dur uint /* in ms */) ITimer{
	real_dur := time.Now().Sub(now) + time.Duration(dur)*time.Millisecond
	timer := time.NewTimer(real_dur)
	log.Infof("real_dur = %v",real_dur)
	done := make(chan bool, 1)
	go func(){
		defer timer.Stop()
		for {
			select {
			case <- done:
				log.Debugf("Timer recv done. dur: %v", dur)
				return
			case t := <- timer.C:
				proc(data, t)
			}
		}
	}()
	itimer := ITimer{timer:timer, done: done}
	return itimer
}

func ticker_create(now time.Time, proc timerProc, data TimerClientData, interval uint /* in ms */) ITicker{
	ticker := time.NewTicker(time.Duration(interval) * time.Millisecond)
	done := make(chan bool, 1)
	go func(){
		defer ticker.Stop()
		for {
			select {
			case <- done:
				log.Debugf("Ticker recv done. interval:%v", interval)
				return
			case t := <- ticker.C:
				proc(data, t)
			}
		}
	}()
	iticker := ITicker{ticker:ticker, done: done}
	return iticker
}