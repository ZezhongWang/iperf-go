# iperf-go
简易版 iperf，通过实现基本的接口之后可以对任意自定义应用层协议进行测速, 使用Go语言实现

# 使用

    $.\iperf-go -h
    Usage of .\iperf-go.exe:
      -D    no delay option
      -P uint
            The number of simultaneous connections (default 1)
      -b uint
            bandwidth limit. (Mb/s)
      -c string
            client side (default "127.0.0.1")
      -d uint
            duration (s) (default 10)
      -debug
            debug mode
      -h    this help
      -i uint
            test interval (ms) (default 1000)
      -l uint
            send/read block size (default 1400)
      -p uint
            connect/listen port (default 5201)
      -proto string
            protocol under test (default "tcp")
      -s    server side

