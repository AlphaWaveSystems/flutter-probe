package config

import (
	"reflect"
	"testing"
	"time"
)

// refFields lists the fields under v (recursively) that hold a map, a slice or a pointer and are not nil: values a shallow copy would share.
func refFields(v reflect.Value, path string, out *[]string) {
	switch v.Kind() {
	case reflect.Map, reflect.Slice, reflect.Pointer:
		if !v.IsNil() {
			*out = append(*out, path)
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			refFields(v.Field(i), path+"."+v.Type().Field(i).Name, out)
		}
	}
}

// Default (and LoadFile, which starts from the same value) copy defaultConfig with a plain assignment. That is a full copy only while the default holds
// no map, slice or pointer: one that did would be shared by every caller, and a write through one would change the next Default. This test fails
// the day a default gains such a field, and says to deep-copy it then.
func TestDefaultConfigHoldsNoSharedReferences(t *testing.T) {
	var shared []string
	refFields(reflect.ValueOf(defaultConfig), "defaultConfig", &shared)
	if len(shared) != 0 {
		t.Errorf("defaultConfig holds values a shallow copy shares: %v: copy them in Default and LoadFile", shared)
	}
}

// What a caller does to the config it got does not reach the next one.
func TestMutatingADefaultDoesNotChangeTheNext(t *testing.T) {
	a := Default()
	a.Project.App = "com.example.changed"
	a.Agent.Port = 1
	a.Defaults.Timeout = time.Second
	a.Env = map[string]string{"X": "1"}
	a.Devices = append(a.Devices, DeviceEntry{})
	b := Default()
	if b.Project.App != "" || b.Agent.Port != 48686 || b.Defaults.Timeout != 30*time.Second || b.Env != nil || b.Devices != nil {
		t.Errorf("the next Default carries what a caller changed: %+v", b.Project)
	}
	if !reflect.DeepEqual(Default(), Default()) {
		t.Error("two Defaults differ")
	}
}
