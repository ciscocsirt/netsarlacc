netsarlacc
===================

netsarlacc is a high performance enterprise HTTP sinkhole designed to be used by corporate SOC or IR teams.  netsarlacc was designed with several purpose-specific goals in mind:

 - Be very fast even under very high load
 - Robust against abuse from malware or broken scripts
 - Provide better logging (in JSON) than traditional webservers like Apache or nginx
 - Allow for a configurable custom response to help users understand that the request was blocked

#### How is netsarlacc intended to be used?
netsarlacc is meant to work in conjuction with existing blocking / captive portal / quarantining / redirecting technologies like DNS RPZ.  In a typical deployment, netsarlacc is the target IP / CNAME provided to clients that look domain names being blocked by your DNS security infrastructure such as DNS RPZ or Cisco's Umbrella.  The logs produced by netsarlacc go beyond the logs available from a typical webserver and were specifically designed with incident response and network monitoring in mind.

#### What's next? How can I help?
At this time, netsarlacc was primarily built for HTTP but we plan on adding support for additional protocols like SMTP soon.  We could use:

- Feedback from the community about the log format and choice of fields to log
- init scripts for various distributions to aid in deployment
- Bug reports

### Requirements
-------------
netsarlacc requires:

- Go version 1.4 or better.  Go version 1.8 (at the time of writing) provides the best performance.
- A Linux server
- Your own TLS certificate / keypair if you want to use TLS


Configuring and using netsarlacc
===================
Once setup, tuned, and tested, netsarlacc is meant to run as a Unix daemon out of an init script or as a service.

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

