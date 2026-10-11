#include "textflag.h"

// func sumAbsPCM16(pcm []byte) uint64
TEXT ·sumAbsPCM16(SB), NOSPLIT, $0-32
	MOVD pcm_base+0(FP), R0
	MOVD pcm_len+8(FP), R1
	LSR $1, R1, R2          // R2 = number of int16 samples

	MOVD $0, R6             // R6 = accumulator (total sum)

loop16:
	CMP $16, R2
	BLT loop8

	VLD1.P 32(R0), [V0.H8, V1.H8]
	VABS V0.H8, V0.H8
	VABS V1.H8, V1.H8

	VUADDLV V0.H8, V2
	VUADDLV V1.H8, V3

	FMOVD F2, R3
	FMOVD F3, R4
	ADD R3, R6
	ADD R4, R6

	SUB $16, R2
	B loop16

loop8:
	CMP $8, R2
	BLT loop1

	VLD1.P 16(R0), [V0.H8]
	VABS V0.H8, V0.H8
	VUADDLV V0.H8, V2
	FMOVD F2, R3
	ADD R3, R6

	SUB $8, R2

loop1:
	CMP $0, R2
	BLE done

	MOVH (R0), R3
	SXTH R3, R3
	CMP $0, R3
	BGE pos
	NEG R3, R3
pos:
	ADD R3, R6
	ADD $2, R0
	SUB $1, R2
	B loop1

done:
	MOVD R6, ret+24(FP)
	RET
