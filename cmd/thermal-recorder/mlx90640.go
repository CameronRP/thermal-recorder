package main

import (
	"encoding/binary"
	"time"

	"github.com/TheCacophonyProject/go-cptv/cptvframe"
	"github.com/x448/float16"
)

func convertRawMLX90640Frame(raw []byte, out *cptvframe.Frame, edgePixels int) error {
	// TODO populate telemetry
	out.Status = cptvframe.Telemetry{
		// Make it appear like there hasn't been a FFC recently. Without
		// this the motion detector will never trigger.
		LastFFCTime: time.Second,
		TimeOn:      time.Minute,
	}

	i := 0
	for y, row := range out.Pix {
		for x := range row {
			out.Pix[y][x] = uint16(float16.Frombits(binary.LittleEndian.Uint16(raw[i:i+2])).Float32()*100) + 27315
			i += 2
		}
	}
	return nil
}
