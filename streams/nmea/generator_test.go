package main

import (
	"math/rand"
	"strconv"
)

// talkers are the talker identifiers of the generated sentences: GPS, GLONASS
// and a combined receiver.
var talkers = []string{"GP", "GL", "GN"}

// sentenceRecord is the streamtest.RecordFunc of an endless NMEA 0183 stream.
// Record i is one GGA, RMC, GSV or VTG sentence, picked at random, at second i
// of a day that wraps at midnight, with plausible values, empty fields where
// receivers leave them empty, a correct checksum and CR LF. The scanner never
// checks the checksum; the generator computes it so the stream looks real.
func sentenceRecord(rng *rand.Rand, i int, dst []byte) []byte {
	start := len(dst)
	dst = append(dst, '$')
	dst = append(dst, talkers[rng.Intn(len(talkers))]...)
	switch rng.Intn(4) {
	case 0:
		dst = gga(rng, i, dst)
	case 1:
		dst = rmc(rng, i, dst)
	case 2:
		dst = gsv(rng, dst)
	default:
		dst = vtg(rng, dst)
	}
	// The checksum is the XOR of the bytes between the start character and
	// the asterisk.
	var sum byte
	for _, b := range dst[start+1:] {
		sum ^= b
	}
	const hex = "0123456789ABCDEF"
	return append(dst, '*', hex[sum>>4], hex[sum&0x0F], '\r', '\n')
}

// gga appends a GGA (fix data) sentence body:
// time, latitude, N/S, longitude, E/W, quality, satellites, HDOP, altitude, M,
// geoid separation, M, and the empty age and station of differential data.
func gga(rng *rand.Rand, i int, dst []byte) []byte {
	dst = append(dst, "GGA,"...)
	dst = utc(i, dst)
	dst = position(rng, dst)
	dst = append(dst, ',')
	dst = strconv.AppendInt(dst, int64(1+rng.Intn(2)), 10)
	dst = append(dst, ',')
	dst = pad(dst, 4+rng.Intn(9), 2)
	dst = append(dst, ',')
	dst = decimal(dst, 5+rng.Intn(25), 1)
	dst = append(dst, ',')
	dst = decimal(dst, rng.Intn(20000), 1)
	dst = append(dst, ",M,"...)
	dst = decimal(dst, 300+rng.Intn(200), 1)
	return append(dst, ",M,,"...)
}

// rmc appends an RMC (recommended minimum) sentence body: time, status,
// latitude, N/S, longitude, E/W, speed in knots, course, date, the empty
// magnetic variation and its direction, and the mode indicator.
func rmc(rng *rand.Rand, i int, dst []byte) []byte {
	dst = append(dst, "RMC,"...)
	dst = utc(i, dst)
	dst = append(dst, ",A"...)
	dst = position(rng, dst)
	dst = append(dst, ',')
	dst = decimal(dst, rng.Intn(1000), 1)
	dst = append(dst, ',')
	dst = decimal(dst, rng.Intn(3600), 1)
	dst = append(dst, ',')
	dst = pad(dst, 1+rng.Intn(28), 2)
	dst = pad(dst, 1+rng.Intn(12), 2)
	dst = pad(dst, rng.Intn(100), 2)
	return append(dst, ",,,A"...)
}

// gsv appends a GSV (satellites in view) sentence body: number of messages,
// message number, satellites in view, and up to four satellites of PRN,
// elevation, azimuth and signal-to-noise ratio. A satellite that is not
// tracked has an empty ratio.
func gsv(rng *rand.Rand, dst []byte) []byte {
	total := 1 + rng.Intn(3)
	dst = append(dst, "GSV,"...)
	dst = strconv.AppendInt(dst, int64(total), 10)
	dst = append(dst, ',')
	dst = strconv.AppendInt(dst, int64(1+rng.Intn(total)), 10)
	dst = append(dst, ',')
	dst = pad(dst, 4*total-rng.Intn(4), 2)
	for n := 1 + rng.Intn(4); n > 0; n-- {
		dst = append(dst, ',')
		dst = pad(dst, 1+rng.Intn(32), 2)
		dst = append(dst, ',')
		dst = pad(dst, rng.Intn(91), 2)
		dst = append(dst, ',')
		dst = pad(dst, rng.Intn(360), 3)
		dst = append(dst, ',')
		if rng.Intn(4) > 0 {
			dst = pad(dst, 10+rng.Intn(40), 2)
		}
	}
	return dst
}

// vtg appends a VTG (track and ground speed) sentence body: true course, T,
// the empty magnetic course, M, speed in knots, N, speed in km/h, K, and the
// mode indicator.
func vtg(rng *rand.Rand, dst []byte) []byte {
	knots := rng.Intn(1000)
	dst = append(dst, "VTG,"...)
	dst = decimal(dst, rng.Intn(3600), 1)
	dst = append(dst, ",T,,M,"...)
	dst = decimal(dst, knots, 1)
	dst = append(dst, ",N,"...)
	dst = decimal(dst, knots*1852/1000, 1)
	return append(dst, ",K,A"...)
}

// utc appends the time of second i of the day as hhmmss.ss.
func utc(i int, dst []byte) []byte {
	s := i % 86400
	dst = pad(dst, s/3600, 2)
	dst = pad(dst, s/60%60, 2)
	dst = pad(dst, s%60, 2)
	return append(dst, ".00"...)
}

// position appends ",ddmm.mmmm,N/S,dddmm.mmmm,E/W".
func position(rng *rand.Rand, dst []byte) []byte {
	dst = append(dst, ',')
	dst = pad(dst, rng.Intn(90), 2)
	dst = pad(dst, rng.Intn(60), 2)
	dst = append(dst, '.')
	dst = pad(dst, rng.Intn(10000), 4)
	dst = append(dst, ',', "NS"[rng.Intn(2)], ',')
	dst = pad(dst, rng.Intn(180), 3)
	dst = pad(dst, rng.Intn(60), 2)
	dst = append(dst, '.')
	dst = pad(dst, rng.Intn(10000), 4)
	return append(dst, ',', "EW"[rng.Intn(2)])
}

// pad appends v in decimal, padded with zeros to width digits.
func pad(dst []byte, v, width int) []byte {
	for d := 10; width > 1; width-- {
		if v < d {
			dst = append(dst, '0')
		}
		d *= 10
	}
	return strconv.AppendInt(dst, int64(v), 10)
}

// decimal appends v/10^frac with frac decimal places.
func decimal(dst []byte, v, frac int) []byte {
	p := 1
	for j := 0; j < frac; j++ {
		p *= 10
	}
	dst = strconv.AppendInt(dst, int64(v/p), 10)
	dst = append(dst, '.')
	return pad(dst, v%p, frac)
}
