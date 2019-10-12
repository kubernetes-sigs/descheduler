// Copyright (c) 2012-2015 Ugorji Nwoke. All rights reserved.
// Use of this source code is governed by a MIT license found in the LICENSE file.

package codec

// All non-std package dependencies live in this file,
// so porting to different environment is easy (just update functions).

func pruneSignExt(v []byte, pos bool) (n int) {
	if len(v) < 2 {
	} else if pos && v[0] == 0 {
		for ; v[n] == 0 && n+1 < len(v) && (v[n+1]&(1<<7) == 0); n++ {
		}
	} else if !pos && v[0] == 0xff {
		for ; v[n] == 0xff && n+1 < len(v) && (v[n+1]&(1<<7) != 0); n++ {
		}
	}
	return
}

// validate that this function is correct ...
// culled from OGRE (Object-Oriented Graphics Rendering Engine)
// function: halfToFloatI (http://stderr.org/doc/ogre-doc/api/OgreBitwise_8h-source.html)
func halfFloatToFloatBits(yy uint16) (d uint32) {
	y := uint32(yy)
	s := (y >> 15) & 0x01
	e := (y >> 10) & 0x1f
	m := y & 0x03ff

	if e == 0 {
		if m == 0 { // plu or minus 0
			return s << 31
		}
		// Denormalized number -- renormalize it
		for (m & 0x00000400) == 0 {
			m <<= 1
			e -= 1
		}
		e += 1
		const zz uint32 = 0x0400
		m &= ^zz
	} else if e == 31 {
		if m == 0 { // Inf
			return (s << 31) | 0x7f800000
		}
		return (s << 31) | 0x7f800000 | (m << 13) // NaN
	}
	e = e + (127 - 15)
	m = m << 13
	return (s << 31) | (e << 23) | m
}

// GrowCap will return a new capacity for a slice, given the following:
//   - oldCap: current capacity
//   - unit: in-memory size of an element
//   - num: number of elements to add
func growCap(oldCap, unit, num int) (newCap int) {
	// appendslice logic (if cap < 1024, *2, else *1.25):
	//   leads to many copy calls, especially when copying bytes.
	//   bytes.Buffer model (2*cap + n): much better for bytes.
	// smarter way is to take the byte-size of the appended element(type) into account

	// maintain 2 thresholds:
	// t1: if cap <= t1, newcap = 2x
	// t2: if cap <= t2, newcap = 1.5x
	//     else          newcap = 1.25x
	//
	// t1, t2 >= 1024 always.
	// This means that, if unit size >= 16, then always do 2x or 1.25x (ie t1, t2, t3 are all same)
	//
	// With this, appending for bytes increase by:
	//    100% up to 4K
	//     75% up to 16K
	//     25% beyond that

	// unit can be 0 e.g. for struct{}{}; handle that appropriately
	if unit <= 0 {
		if uint64(^uint(0)) == ^uint64(0) { // 64-bit
			var maxInt64 uint64 = 1<<63 - 1 // prevent failure with overflow int on 32-bit (386)
			return int(maxInt64)            // math.MaxInt64
		}
		return 1<<31 - 1 //  math.MaxInt32
	}

	// handle if num < 0, cap=0, etc.

	var t1, t2 int // thresholds
	if unit <= 4 {
		t1, t2 = 4*1024, 16*1024
	} else if unit <= 16 {
		t1, t2 = unit*1*1024, unit*4*1024
	} else {
		t1, t2 = 1024, 1024
	}

	if oldCap <= 0 {
		newCap = 2
	} else if oldCap <= t1 { // [0,t1]
		newCap = oldCap * 8 / 4
	} else if oldCap <= t2 { // (t1,t2]
		newCap = oldCap * 6 / 4
	} else { // (t2,infinity]
		newCap = oldCap * 5 / 4
	}

	if num > 0 && newCap < num+oldCap {
		newCap = num + oldCap
	}

	// ensure newCap takes multiples of a cache line (size is a multiple of 64)
	t1 = newCap * unit
	t2 = t1 % 64
	if t2 != 0 {
		t1 += 64 - t2
		newCap = t1 / unit
	}

	return
}
