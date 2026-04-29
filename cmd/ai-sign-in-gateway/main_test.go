package main

import (
	"net"
	"strconv"
	"testing"
)

func TestListenWithPortOffsetSkipsOccupiedPort(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("occupy port: %v", err)
	}
	defer occupied.Close()

	startPort := occupied.Addr().(*net.TCPAddr).Port
	listener, actualPort, err := listenWithPortOffset("127.0.0.1", startPort, maxDesktopPortOffset)
	if err != nil {
		t.Fatalf("listen with offset: %v", err)
	}
	defer listener.Close()

	if actualPort == startPort {
		t.Fatalf("expected port offset, got same port %d", actualPort)
	}
	if actualPort < startPort || actualPort > startPort+maxDesktopPortOffset {
		t.Fatalf("actual port %d outside expected range [%d, %d]", actualPort, startPort, startPort+maxDesktopPortOffset)
	}
}

func TestDefaultPortOwnerReportsCurrentProgramAndFreePort(t *testing.T) {
	if got := defaultPortOwner("127.0.0.1", 8972, 8972); got != "当前程序" {
		t.Fatalf("defaultPortOwner current program = %q", got)
	}

	free, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve free port: %v", err)
	}
	port := free.Addr().(*net.TCPAddr).Port
	if err := free.Close(); err != nil {
		t.Fatalf("close free port listener: %v", err)
	}

	if got := defaultPortOwner("127.0.0.1", port, port+1); got != "未占用" {
		t.Fatalf("defaultPortOwner free port = %q for port %s", got, strconv.Itoa(port))
	}
}
