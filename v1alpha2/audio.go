package assistant

import (
	gassist "google.golang.org/genproto/googleapis/assistant/embedded/v1alpha2"
)

//AudioSettings holds the settings for an audio buffer
type AudioSettings struct {
	AudioInEncoding        gassist.AudioInConfig_Encoding
	AudioInSampleRateHertz int32

	AudioOutEncoding         gassist.AudioOutConfig_Encoding
	AudioOutSampleRateHertz  int32
	AudioOutVolumePercentage int32
}

// NewAudioSettings returns a new audio settings object for configuring the Assistant
// AudioInConfig_Encoding enum:
// - 0: Encoding unspecified, Google will return result [google.rpc.Code.INVALID_ARGUMENT][]
// - 1: Signed 16-bit little-endian linear PCM
// - 2: FLAC, supports 16-bit and 24-bit samples and includes the FLAC stream header followed by audio data
// AudioOutConfig_Encoding enum:
// - 0: Encoding unspecified, Google will return result [google.rpc.Code.INVALID_ARGUMENT][]
// - 1: Signed 16-bit little-endian linear PCM
// - 2: MP3 audio encoding, sample rate is encoded in the payload
// - 3: Opus audio encoding wrapped in an OGG container, sample rate is encoded in the payload
func NewAudioSettings(audioInEncoding gassist.AudioInConfig_Encoding, audioOutEncoding gassist.AudioOutConfig_Encoding, audioInSampleRateHertz, audioOutSampleRateHertz, audioOutVolumePercentage int32) *AudioSettings {
	return &AudioSettings{
		AudioInEncoding:          audioInEncoding,
		AudioInSampleRateHertz:   audioInSampleRateHertz,
		AudioOutEncoding:         audioOutEncoding,
		AudioOutSampleRateHertz:  audioOutSampleRateHertz,
		AudioOutVolumePercentage: audioOutVolumePercentage,
	}
}
