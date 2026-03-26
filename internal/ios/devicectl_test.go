package ios

import (
	"testing"
)

func TestNewDeviceCtl(t *testing.T) {
	d := NewDeviceCtl()
	if d == nil {
		t.Fatal("NewDeviceCtl() returned nil")
	}
}

func TestListPhysicalDevices_NoDevices(t *testing.T) {
	// ListPhysicalDevices should not panic even if no devices are connected.
	// It may return an error if idevice_id is not installed, which is acceptable.
	_, _ = ListPhysicalDevices(t.Context())
}

func TestNewSimCtl(t *testing.T) {
	s := New()
	if s == nil {
		t.Fatal("New() returned nil")
	}
}
