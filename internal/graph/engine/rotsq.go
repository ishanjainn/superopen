package engine

import (
	"encoding/binary"

	"github.com/zeebo/xxh3"
)

const (
	rotSQInputDimensions = 768
	rotSQDimensions      = 1024
	rotSQCodeBytes       = rotSQDimensions / 2
	rotSQLevels          = 15
)

type rotSQCode struct {
	Codes   [rotSQCodeBytes]byte
	Scale   float32
	Offset  float32
	CodeSum int32
}

var rotSQDiagonal = makeRotSQDiagonal()

func makeRotSQDiagonal() [rotSQDimensions]float32 {
	var diagonal [rotSQDimensions]float32
	for dimension := range diagonal {
		var encoded [4]byte
		binary.LittleEndian.PutUint32(encoded[:], uint32(dimension))
		if xxh3.HashSeed(encoded[:], 0x5bd1e995)&1 != 0 {
			diagonal[dimension] = 1
		} else {
			diagonal[dimension] = -1
		}
	}
	return diagonal
}

func encodeRotSQ(vector []float32) rotSQCode {
	var rotated [rotSQDimensions]float32
	for dimension := 0; dimension < len(vector) && dimension < rotSQInputDimensions; dimension++ {
		rotated[dimension] = vector[dimension] * rotSQDiagonal[dimension]
	}
	fastWalshHadamard(rotated[:])
	const inverseSqrtDimensions float32 = 1.0 / 32.0
	low := rotated[0] * inverseSqrtDimensions
	high := low
	for dimension := range rotated {
		rotated[dimension] *= inverseSqrtDimensions
		if rotated[dimension] < low {
			low = rotated[dimension]
		}
		if rotated[dimension] > high {
			high = rotated[dimension]
		}
	}
	step := float32(1)
	if valueRange := high - low; valueRange > 0 {
		step = valueRange / rotSQLevels
	}
	code := rotSQCode{Offset: low, Scale: step}
	for dimension, value := range rotated {
		quantized := int32((value-low)/step + 0.5)
		if quantized < 0 {
			quantized = 0
		}
		if quantized > rotSQLevels {
			quantized = rotSQLevels
		}
		code.CodeSum += quantized
		if dimension&1 != 0 {
			code.Codes[dimension>>1] |= byte(quantized << 4)
		} else {
			code.Codes[dimension>>1] |= byte(quantized)
		}
	}
	return code
}

func fastWalshHadamard(vector []float32) {
	for length := 1; length < len(vector); length <<= 1 {
		for start := 0; start < len(vector); start += length << 1 {
			for index := start; index < start+length; index++ {
				left, right := vector[index], vector[index+length]
				vector[index] = left + right
				vector[index+length] = left - right
			}
		}
	}
}

func rotSQInnerProduct(left, right rotSQCode) float32 {
	var dot int64
	for index := range left.Codes {
		leftByte, rightByte := left.Codes[index], right.Codes[index]
		dot += int64(leftByte&0x0f) * int64(rightByte&0x0f)
		dot += int64(leftByte>>4) * int64(rightByte>>4)
	}
	dimensions := float64(rotSQDimensions)
	product := dimensions*float64(left.Offset)*float64(right.Offset) +
		float64(left.Offset)*float64(right.Scale)*float64(right.CodeSum) +
		float64(right.Offset)*float64(left.Scale)*float64(left.CodeSum) +
		float64(left.Scale)*float64(right.Scale)*float64(dot)
	return float32(product)
}

func decodeRotSQ(code rotSQCode) [rotSQDimensions]float32 {
	var result [rotSQDimensions]float32
	for index, packed := range code.Codes {
		result[index*2] = code.Offset + code.Scale*float32(packed&0x0f)
		result[index*2+1] = code.Offset + code.Scale*float32(packed>>4)
	}
	return result
}
