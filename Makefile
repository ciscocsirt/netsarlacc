CC=gccgo
GO=go

# Generic GCC performance
#CFLAGS=-Wall -Wextra -march=native -O2

# Generic GCC debugging
#CFLAGS=-Wall -Wextra -O0 -g

# Graphite loop stuff
CFLAGS=-Wall -Wextra -march=native -O2 -floop-interchange -fgraphite-identity -floop-block -floop-strip-mine

GOFILES=netsarlacc.go dispatcher.go logger.go worker.go

main: gobuild

gccgo: $(GOFILES)
	$(CC) $(CFLAGS) -o netsarlacc $(GOFILES)

gobuild: $(GOFILES)
	$(GO) build $(GOFILES)

clean:
	rm -f sinkhole-*.log
	rm -f netsarlacc
	rm -f *.o
	rm -f *~
	rm -f \#*
