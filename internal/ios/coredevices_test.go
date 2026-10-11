package ios

import "testing"

const coreDevicesJSON = `{"result":{"devices":[
 {"identifier":"AAAAAAAA-0000-0000-0000-000000000001",
  "connectionProperties":{"pairingState":"paired","transportType":"localNetwork","tunnelState":"disconnected"},
  "deviceProperties":{"name":"Test Phone","osVersionNumber":"18.6","developerModeStatus":"enabled"},
  "hardwareProperties":{"udid":"00008000-000000000000001E","deviceType":"iPhone","platform":"iOS","marketingName":"iPhone 15"}},
 {"identifier":"AAAAAAAA-0000-0000-0000-000000000002",
  "connectionProperties":{"pairingState":"paired","transportType":"wired","tunnelState":"connected"},
  "deviceProperties":{"name":"Test Pad","osVersionNumber":"17.5","developerModeStatus":"disabled"},
  "hardwareProperties":{"udid":"00008000-000000000000002E","deviceType":"iPad","platform":"iOS","marketingName":"iPad Air"}},
 {"identifier":"AAAAAAAA-0000-0000-0000-000000000003",
  "connectionProperties":{"pairingState":"unpaired"},
  "deviceProperties":{"name":"Watch","osVersionNumber":"11.0"},
  "hardwareProperties":{"udid":"00008000-000000000000003E","deviceType":"appleWatch","platform":"watchOS","marketingName":"Apple Watch"}},
 {"identifier":"AAAAAAAA-0000-0000-0000-000000000004",
  "connectionProperties":{"pairingState":"unpaired"},
  "deviceProperties":{"name":"No udid"},"hardwareProperties":{"deviceType":"iPhone"}}
]}}`

func TestParseCoreDevices(t *testing.T) {
	got, err := ParseCoreDevices([]byte(coreDevicesJSON))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("only iPhones and iPads with a UDID are kept, got %+v", got)
	}
	p := got[0]
	if p.UDID != "00008000-000000000000001E" || p.Name != "Test Phone" || p.Model != "iPhone 15" || p.OSVersion != "iOS 18.6" || !p.Paired || p.Transport != "localNetwork" || p.Tunnel != "disconnected" || p.DevMode != "enabled" {
		t.Errorf("phone = %+v", p)
	}
	if q := got[1]; q.Transport != "wired" || q.DevMode != "disabled" || q.Model != "iPad Air" {
		t.Errorf("pad = %+v", q)
	}
	if _, err := ParseCoreDevices([]byte("not json")); err == nil {
		t.Error("garbage must be an error")
	}
	if got, err := ParseCoreDevices([]byte(`{"result":{}}`)); err != nil || len(got) != 0 {
		t.Errorf("empty = %v %v", got, err)
	}
}
