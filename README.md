netsarlacc
===================

netsarlacc is a high performance enterprise HTTP (and SMTP) sinkhole designed to be used by corporate SOC or IR teams.  netsarlacc was designed with several purpose-specific goals in mind:

 - Be very fast even under very high load
 - Robust against abuse from malware or broken scripts
 - Provide better logging (in JSON) than traditional webservers like Apache or nginx
 - Allow for a configurable custom response to help users understand that the request was blocked

#### How is netsarlacc intended to be used?
netsarlacc is meant to work in conjunction with existing blocking / captive portal / quarantining / redirecting technologies like DNS RPZ.  In a typical deployment, netsarlacc is the target IP / CNAME provided to clients that look domain names being blocked by your DNS security infrastructure such as DNS RPZ or Cisco's Umbrella.  The logs produced by netsarlacc go beyond the logs available from a typical webserver and were specifically designed with incident response and network monitoring in mind.

#### What's next? How can I help?
At this time, netsarlacc was primarily built for HTTP but basic support for non-interactive SMTP is available too.  Later more interactivity for SMTP and other protocols may be added.

- Feedback from the community about the log format and choice of fields to log
- init scripts for various distributions to aid in deployment
- Bug reports

### Requirements
-------------
netsarlacc requires:

- Go version 1.4 or better.  Go version 1.8 (at the time of writing) provides the best performance.
- A Linux server
- Your own TLS certificate / keypair if you want to use TLS


Building, configuring, and using netsarlacc
===================
netsarlacc can be built with go or gccgo.  Testing of Go 1.8 vs GCC 5.4.0 shows Go is significantly faster.  This is likely due to gccgo using and older version of the go language specification.

You can build netsarlacc with the provided makefile:
```
# make
go build netsarlacc.go dispatcher.go logger.go worker.go
```

**For initial testing a small configuration file like the following is a good start:**
```
$ cat lambda.json
{
    "Daemonize"       : false,
    "Workers"         : 4,
    "WorkingDirectory": "/home/brenrigh/projects/github/netsarlacc",
    "LogDirectory"    : ".",
    "ListenList"      : [{"Host":"0.0.0.0", "Port":"8080", "Proto":"tcp", "App":"http", "TLS":false},
                         {"Host":"0.0.0.0", "Port":"8443", "Proto":"tcp", "App":"http", "TLS":true}
                        ]
}
```

**Then you can start the sinkhole using this configuration file:**
```
$ ./netsarlacc -c /home/brenrigh/projects/github/netsarlacc/config.json
Starting sinkhole instance netsarlacc-lambda
Started with 8 max threads
Now running with with 8 max threads
Starting 512 readers
Starting 4 workers
Listening on 0.0.0.0 tcp/8080
Listening on 0.0.0.0 tcp/8443
Wrapped 0.0.0.0 tcp/8443 with TLS
Listening on 0.0.0.0 tcp/8444
Wrapped 0.0.0.0 tcp/8444 with TLS
```

To test the performance of netsarlacc, ApacheBench is one of the easier tools to use:
```
$ ab -l -q -n 200000 -c 80 http://127.0.0.1:8080/netsarlacc_test
```

Once setup, tuned, and tested, netsarlacc is meant to run as a Unix daemon out of an init script or as a service.


### Log format
netsarlacc logs use JSON which is easily parsed by many tools. Logs entries are written as requests come in and the log files are rotated every 10 minutes.

For human consumption, the tool `jq` can be used to make easier to read:

```
$ cat sinkhole-2017-06-12-22-50-UTC.log | jq . | less
```

Which will look something like:
```
{
  "timestamp": "2017-06-12 22:57:45.259795 +0000 UTC",
  "bytes_client": "97",
  "http_method": "GET",
  "url_path": "/netsarlacc_test",
  "http_version": "HTTP/1.0",
  "http_user_agent": "ApacheBench/2.3",
  "dst_name": "127.0.0.1:8080",
  "src_ip": "127.0.0.1",
  "src_port": "39670",
  "sinkhole_instance": "netsarlacc-lambda",
  "sinkhole_addr": "0.0.0.0",
  "sinkhole_port": "8080",
  "sinkhole_proto": "tcp",
  "sinkhole_app": "http",
  "sinkhole_tls": false,
  "raw_data": "474554202f6e65747361726c6163635f7465737420485454502f312e300d0a486f73743a203132372e302e302e313a383038300d0a557365722d4167656e743a2041706163686542656e63682f322e330d0a4163636570743a202a2f2a0d0a0d0a",
  "request_error": false
}
```

### Performance tuning
Benchmarking and profiling netsarlacc shows CPU usage roughly breaks down like so:

- 25% calling accept() on the listening sockets (almost entirely kernel time)
- 25% filling out the response template
- 25% calling write() on each socket to send the template (almost entirely kernel time)
- 25% everything else

Testing shows that if there are too few workers the bottleneck is in filling out the templates and if there are too many workers, they star	ve the other tasks of CPU time.  The ideal number of workers will vary from machine to machine but the best performance seems to come from matching up the number of workers with the number of physical CPU cores.  If you have a hyperthreaded machine (SMT) this will be half the number of CPUs your operating system sees.  For maximum performance the number of readers should be between 1x and 2x the number of workers.  In a real-world deployment though, you want many more readers than workers (around 64x) to handle abusive / broken clients that hold onto sockets instead of making a valid HTTP request.  If you don't have enough readers, a single machine performing a Slowloris-style attack can tie up all of the readers.

Because so much time is spent in the kernel, it's important that the kernel is configured to balance TCP connections across multiple CPUs.  Your Ethernet card may have multiple receive queues and you can try to distribute incoming connections across those receive queues and then assign each queue to a specific CPU.  An easier alternative is to just tell each receive queues that they can use all the CPUs.

Suppose your Ethernet card is `eth1` then you can run:
```
# find /sys/class/net/eth1/queues/ | egrep rps_cpus | while read LINE; do echo ffff > $LINE; done
```

Occasionally Go must run through a garbage collection sweep which can cause short pauses in calling accept() for new connections.  At very high connection rates it is possible to fill up the operating system's outstanding connection queue and additional connections will get an RST back from the operating system until netsarlacc can catch up.  This will likely only come up during benchmarking or at maximum load but if tuning is needed, check out the documentation for:

```
/proc/sys/net/core/netdev_max_backlog
/proc/sys/net/ipv4/tcp_max_syn_backlog
```
