package main

import (
	"crypto/rand"
	"math/big"
)

// generateVerificationCode returns a 4-digit int code with no repeated digits.
func generateVerificationCode() (int, error) {
	digits := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}

	// Fisher-Yates shuffle
	for i := len(digits) - 1; i > 0; i-- {
		j, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return 0, err
		}
		digits[i], digits[j.Int64()] = digits[j.Int64()], digits[i]
	}

	code := digits[0]*1000 + digits[1]*100 + digits[2]*10 + digits[3]
	return code, nil
}
