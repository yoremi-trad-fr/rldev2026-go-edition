// Package nwaaudio converts RealLive NWA audio into common PCM/MP3 formats.
package nwaaudio

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"

	shinemp3 "github.com/braheezy/shine-mp3/pkg/mp3"
	"github.com/hasenbanck/nwa"
)

const mp3Bitrate = 128

// Info describes decoded NWA audio properties.
type Info struct {
	Channels      int
	BitsPerSample int
	Frequency     int
}

// ConvertFile decodes a NWA file and writes it as WAV or MP3.
func ConvertFile(inputPath, outputPath, format string) (Info, error) {
	in, err := os.Open(inputPath)
	if err != nil {
		return Info{}, err
	}
	defer in.Close()

	nf, err := nwa.NewNwaFile(in)
	if err != nil {
		return Info{}, err
	}

	info := Info{
		Channels:      nf.Channels,
		BitsPerSample: nf.Bps,
		Frequency:     nf.Freq,
	}

	wavData, err := io.ReadAll(nf)
	if err != nil {
		return Info{}, err
	}
	if len(wavData) < 44 {
		return Info{}, fmt.Errorf("decoded WAV is too small")
	}

	switch strings.ToLower(strings.TrimSpace(format)) {
	case "wav":
		return info, os.WriteFile(outputPath, wavData, 0644)
	case "mp3":
		if shinemp3.CheckConfig(info.Frequency, mp3Bitrate) < 0 {
			return Info{}, fmt.Errorf("unsupported MP3 settings: %d Hz at %d kbps", info.Frequency, mp3Bitrate)
		}
		samples, err := pcmToInt16(wavData[44:], info.BitsPerSample)
		if err != nil {
			return Info{}, err
		}
		out, err := os.Create(outputPath)
		if err != nil {
			return Info{}, err
		}
		defer out.Close()
		return info, shinemp3.NewEncoder(info.Frequency, info.Channels).Write(out, samples)
	default:
		return Info{}, fmt.Errorf("unknown audio format %q", format)
	}
}

func pcmToInt16(pcm []byte, bitsPerSample int) ([]int16, error) {
	switch bitsPerSample {
	case 8:
		samples := make([]int16, len(pcm))
		for i, b := range pcm {
			samples[i] = int16((int(b) - 128) << 8)
		}
		return samples, nil
	case 16:
		if len(pcm)%2 != 0 {
			return nil, fmt.Errorf("odd 16-bit PCM data length")
		}
		samples := make([]int16, len(pcm)/2)
		for i := range samples {
			samples[i] = int16(binary.LittleEndian.Uint16(pcm[i*2:]))
		}
		return samples, nil
	default:
		return nil, fmt.Errorf("unsupported PCM bit depth: %d", bitsPerSample)
	}
}
