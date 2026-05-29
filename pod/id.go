package pod

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"time"
)

// Pod and principal identifiers are prefixed ULIDs (domain-model §7): a stable
// prefix plus a 26-char Crockford base32 ULID. IDs are immutable and never
// reused (pod-spec §3).
const (
	podIDPrefix = "pod_"
	ulidLen     = 26
	crockford   = "0123456789ABCDEFGHJKMNPQRSTVWXYZ" // excludes I L O U
)

// crockfordSet is the membership set for validation.
var crockfordSet = func() [256]bool {
	var s [256]bool
	for i := range len(crockford) {
		s[crockford[i]] = true
	}
	return s
}()

// NewPodID returns a fresh immutable Pod identifier, "pod_" + ULID. The ULID
// encodes a 48-bit millisecond timestamp and 80 bits of randomness, so IDs are
// unique and roughly time-ordered.
func NewPodID() (string, error) {
	u, err := newULID(time.Now())
	if err != nil {
		return "", err
	}
	return podIDPrefix + u, nil
}

// IsValidPodID reports whether s is a well-formed Pod identifier.
func IsValidPodID(s string) bool {
	if len(s) != len(podIDPrefix)+ulidLen {
		return false
	}
	if s[:len(podIDPrefix)] != podIDPrefix {
		return false
	}
	return isULID(s[len(podIDPrefix):])
}

func isULID(s string) bool {
	if len(s) != ulidLen {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !crockfordSet[s[i]] {
			return false
		}
	}
	return true
}

// newULID builds a 16-byte ULID (6-byte ms timestamp + 10-byte entropy) and
// renders it as 26 Crockford base32 characters.
func newULID(t time.Time) (string, error) {
	var id [16]byte
	ms := uint64(t.UnixMilli())
	// 48-bit big-endian timestamp in id[0:6].
	var tb [8]byte
	binary.BigEndian.PutUint64(tb[:], ms)
	copy(id[0:6], tb[2:8])
	if _, err := rand.Read(id[6:]); err != nil {
		return "", fmt.Errorf("ulid entropy: %w", err)
	}
	return encodeULID(id), nil
}

// encodeULID renders 16 bytes as the canonical 26-char Crockford base32 ULID.
func encodeULID(id [16]byte) string {
	e := crockford
	d := make([]byte, ulidLen)
	d[0] = e[(id[0]&224)>>5]
	d[1] = e[id[0]&31]
	d[2] = e[(id[1]&248)>>3]
	d[3] = e[((id[1]&7)<<2)|((id[2]&192)>>6)]
	d[4] = e[(id[2]&62)>>1]
	d[5] = e[((id[2]&1)<<4)|((id[3]&240)>>4)]
	d[6] = e[((id[3]&15)<<1)|((id[4]&128)>>7)]
	d[7] = e[(id[4]&124)>>2]
	d[8] = e[((id[4]&3)<<3)|((id[5]&224)>>5)]
	d[9] = e[id[5]&31]
	d[10] = e[(id[6]&248)>>3]
	d[11] = e[((id[6]&7)<<2)|((id[7]&192)>>6)]
	d[12] = e[(id[7]&62)>>1]
	d[13] = e[((id[7]&1)<<4)|((id[8]&240)>>4)]
	d[14] = e[((id[8]&15)<<1)|((id[9]&128)>>7)]
	d[15] = e[(id[9]&124)>>2]
	d[16] = e[((id[9]&3)<<3)|((id[10]&224)>>5)]
	d[17] = e[id[10]&31]
	d[18] = e[(id[11]&248)>>3]
	d[19] = e[((id[11]&7)<<2)|((id[12]&192)>>6)]
	d[20] = e[(id[12]&62)>>1]
	d[21] = e[((id[12]&1)<<4)|((id[13]&240)>>4)]
	d[22] = e[((id[13]&15)<<1)|((id[14]&128)>>7)]
	d[23] = e[(id[14]&124)>>2]
	d[24] = e[((id[14]&3)<<3)|((id[15]&224)>>5)]
	d[25] = e[id[15]&31]
	return string(d)
}
