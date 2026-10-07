#include "textflag.h"

// func dotProdF32(a, b []float32, l int) float32
TEXT ·dotProdF32(SB), NOSPLIT, $0-60
	MOVD a_base+0(FP), R0
	MOVD b_base+24(FP), R1
	MOVD l+48(FP), R2

	FMOVS $0.0, F0
	FMOVS $0.0, F1
	FMOVS $0.0, F2
	FMOVS $0.0, F3

	CMP $0, R2
	BLE done

loop4:
	CMP $4, R2
	BLT loop1

	FMOVS 0(R0), F4
	FMOVS 0(R1), F5
	FMULS F5, F4
	FADDS F4, F0

	FMOVS 4(R0), F6
	FMOVS 4(R1), F7
	FMULS F7, F6
	FADDS F6, F1

	FMOVS 8(R0), F8
	FMOVS 8(R1), F9
	FMULS F9, F8
	FADDS F8, F2

	FMOVS 12(R0), F10
	FMOVS 12(R1), F11
	FMULS F11, F10
	FADDS F10, F3

	ADD $16, R0
	ADD $16, R1
	SUB $4, R2
	B loop4

loop1:
	CMP $0, R2
	BLE reduce

	FMOVS 0(R0), F4
	FMOVS 0(R1), F5
	FMULS F5, F4
	FADDS F4, F0

	ADD $4, R0
	ADD $4, R1
	SUB $1, R2
	B loop1

reduce:
	FADDS F1, F0
	FADDS F2, F0
	FADDS F3, F0

done:
	FMOVS F0, ret+56(FP)
	RET
