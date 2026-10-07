#include "textflag.h"

// func fftButterfly2_arm64(out, sub0, sub1, tw []cpx, m int)
TEXT ·fftButterfly2_arm64(SB), NOSPLIT, $0-104
	MOVD out_base+0(FP), R0
	MOVD sub0_base+24(FP), R1
	MOVD sub1_base+48(FP), R2
	MOVD tw_base+72(FP), R3
	MOVD m+96(FP), R4

	// R5 = m * 8 (byte distance between out[k] and out[m+k])
	LSL $3, R4, R5

loop2:
	CMP $2, R4
	BLT loop1

	// --- 2 butterflies (16 bytes = 2 complex numbers) ---
	FMOVS 0(R2), F0   // b0_re
	FMOVS 4(R2), F1   // b0_im
	FMOVS 8(R2), F2   // b1_re
	FMOVS 12(R2), F3  // b1_im

	FMOVS 0(R3), F4   // w0_re
	FMOVS 4(R3), F5   // w0_im
	FMOVS 8(R3), F6   // w1_re
	FMOVS 12(R3), F7  // w1_im

	FMOVS 0(R1), F8   // a0_re
	FMOVS 4(R1), F9   // a0_im
	FMOVS 8(R1), F10  // a1_re
	FMOVS 12(R1), F11 // a1_im

	// t0_re = b0_re*w0_re - b0_im*w0_im
	FMULS F4, F0, F12
	FMULS F5, F1, F13
	FSUBS F13, F12, F12

	// t0_im = b0_re*w0_im + b0_im*w0_re
	FMULS F5, F0, F14
	FMULS F4, F1, F15
	FADDS F15, F14, F14

	// t1_re = b1_re*w1_re - b1_im*w1_im
	FMULS F6, F2, F16
	FMULS F7, F3, F17
	FSUBS F17, F16, F16

	// t1_im = b1_re*w1_im + b1_im*w1_re
	FMULS F7, F2, F18
	FMULS F6, F3, F19
	FADDS F19, F18, F18

	// out0_lo = a0 + t0
	FADDS F12, F8, F20
	FADDS F14, F9, F21

	// out1_lo = a1 + t1
	FADDS F16, F10, F22
	FADDS F18, F11, F23

	// out0_hi = a0 - t0
	FSUBS F12, F8, F24
	FSUBS F14, F9, F25

	// out1_hi = a1 - t1
	FSUBS F16, F10, F26
	FSUBS F18, F11, F27

	// Store out_lo
	FMOVS F20, 0(R0)
	FMOVS F21, 4(R0)
	FMOVS F22, 8(R0)
	FMOVS F23, 12(R0)

	// Store out_hi at R0 + R5
	ADD R0, R5, R6
	FMOVS F24, 0(R6)
	FMOVS F25, 4(R6)
	FMOVS F26, 8(R6)
	FMOVS F27, 12(R6)

	ADD $16, R0
	ADD $16, R1
	ADD $16, R2
	ADD $16, R3
	SUB $2, R4
	B loop2

loop1:
	CMP $1, R4
	BLT done

	// --- 1 scalar butterfly ---
	FMOVS 0(R2), F0   // b_re
	FMOVS 4(R2), F1   // b_im
	FMOVS 0(R3), F4   // w_re
	FMOVS 4(R3), F5   // w_im
	FMOVS 0(R1), F8   // a_re
	FMOVS 4(R1), F9   // a_im

	// t_re = b_re*w_re - b_im*w_im
	FMULS F4, F0, F12
	FMULS F5, F1, F13
	FSUBS F13, F12, F12

	// t_im = b_re*w_im + b_im*w_re
	FMULS F5, F0, F14
	FMULS F4, F1, F15
	FADDS F15, F14, F14

	// out_lo = a + t
	FADDS F12, F8, F20
	FADDS F14, F9, F21

	// out_hi = a - t
	FSUBS F12, F8, F24
	FSUBS F14, F9, F25

	FMOVS F20, 0(R0)
	FMOVS F21, 4(R0)

	ADD R0, R5, R6
	FMOVS F24, 0(R6)
	FMOVS F25, 4(R6)

done:
	RET
