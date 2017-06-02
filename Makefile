CC=gccgo
GO=go

# Generic GCC performance
#CFLAGS=-Wall -Wextra -march=native -O2

# Generic GCC debugging
#CFLAGS=-Wall -Wextra -O0 -g

# Graphite loop stuff
CFLAGS=-Wall -Wextra -march=native -O2 -floop-interchange -fgraphite-identity -floop-block -floop-strip-mine

GOFILES=collector.go dispatcher.go logger.go worker.go netsarlacc.go

main: gccgo

gccgo: $(GOFILES)
	$(CC) $(CFLAGS) -o netsarlacc $(GOFILES)

gorun: $(GOFILES)
	$(GO) run $(GOFILES)

clean:
	rm -f siknhole-*.log
	rm -f netsarlacc
	rm -f *.o
	rm -f *~
	rm -f \#*
