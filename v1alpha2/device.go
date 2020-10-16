package assistant

import (
	gassist "google.golang.org/genproto/googleapis/assistant/embedded/v1alpha2"
)

// Device holds a Google Assistant device
type Device struct {
	*gassist.DeviceConfig
}

// NewDevice returns a new device object for configuring the Assistant
func NewDevice(deviceID, deviceModelID string) *Device {
	return &Device{
		&gassist.DeviceConfig{
			DeviceId:      deviceID,
			DeviceModelId: deviceModelID,
		},
	}
}
