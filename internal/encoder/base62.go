package encoder

const alphabet = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
const base = uint64(len(alphabet))

// Encode converts a number to a base62 string
func Encode(num uint64) string {
	if num == 0 {
		return string(alphabet[0])
	}

	encoded := ""
	for num > 0 {
		remainder := num % base
		encoded = string(alphabet[remainder]) + encoded
		num = num / base
	}

	return encoded
}

// Decode converts a base62 string back to a number
func Decode(encoded string) uint64 {
	var num uint64 = 0

	for _, char := range encoded {
		num = num * base
		num += uint64(indexOf(byte(char)))
	}

	return num
}

func indexOf(char byte) int {
	for i, c := range []byte(alphabet) {
		if c == char {
			return i
		}
	}
	return -1
}
