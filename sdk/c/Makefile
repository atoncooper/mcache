# mcache C SDK Makefile

CC      ?= gcc
AR      ?= ar
CFLAGS  ?= -Wall -Wextra -O2 -std=c11
PREFIX  ?= /usr/local

SRCDIR  := src
INCDIR  := include
OBJDIR  := build
LIBDIR  := lib

SRCS    := $(wildcard $(SRCDIR)/*.c)
OBJS    := $(patsubst $(SRCDIR)/%.c,$(OBJDIR)/%.o,$(SRCS))

UNAME_S := $(shell uname -s 2>/dev/null || echo Windows)

ifeq ($(UNAME_S),Linux)
    LDFLAGS += -lpthread
    SOEXT   := so
    SOFLAGS := -shared -fPIC
endif
ifeq ($(UNAME_S),Darwin)
    SOEXT   := dylib
    SOFLAGS := -dynamiclib
endif
ifeq ($(findstring MINGW,$(UNAME_S)),MINGW)
    LDFLAGS += -lws2_32
    SOEXT   := dll
    SOFLAGS := -shared
endif

.PHONY: all clean install test

all: $(LIBDIR)/libmcache.a $(LIBDIR)/libmcache.$(SOEXT)

$(OBJDIR)/%.o: $(SRCDIR)/%.c
	@mkdir -p $(OBJDIR)
	$(CC) $(CFLAGS) -I$(INCDIR) -I$(SRCDIR) -c $< -o $@

$(LIBDIR)/libmcache.a: $(OBJS)
	@mkdir -p $(LIBDIR)
	$(AR) rcs $@ $^

$(LIBDIR)/libmcache.$(SOEXT): $(OBJS)
	@mkdir -p $(LIBDIR)
	$(CC) $(SOFLAGS) -o $@ $^ $(LDFLAGS)

clean:
	rm -rf $(OBJDIR) $(LIBDIR)

install:
	install -d $(PREFIX)/lib $(PREFIX)/include/mcache
	install -m 644 $(LIBDIR)/libmcache.a $(PREFIX)/lib/
	install -m 755 $(LIBDIR)/libmcache.$(SOEXT) $(PREFIX)/lib/
	install -m 644 $(INCDIR)/mcache/mcache.h $(PREFIX)/include/mcache/

# Build and run tests
test: $(LIBDIR)/libmcache.a
	$(CC) $(CFLAGS) -I$(INCDIR) -I$(SRCDIR) -Itests \
		tests/test_protocol.c tests/test_client.c \
		-L$(LIBDIR) -lmcache_c $(LDFLAGS) -o $(OBJDIR)/mcache_c_test
	$(OBJDIR)/mcache_c_test
