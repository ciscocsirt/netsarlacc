#! /bin/bash

# Source function library.
. /etc/init.d/functions

start() {
    start-stop-daemon -S -n netsarlacc -a /usr/bin/netsarlacc --daemonize true
}

stop() {
    start-stop-daemon -K -n netsarlacc /usr/bin/netsarlacc
}


case "$1" in
  start)
    echo "Starting netsarlacc"
    start
    ;;
  stop)
    echo "Stopping netsarlacc"
    stop
    ;;
  *)
    echo "Usage: $0 {start|stop}"
  exit 1
esac

exit 0