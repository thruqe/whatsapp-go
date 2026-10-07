#include "textflag.h"

// func fftButterfly2_amd64(out, sub0, sub1, tw []cpx, m int)
TEXT ·fftButterfly2_amd64(SB), NOSPLIT, $0-104
	MOVQ out_base+0(FP), R8
	MOVQ sub0_base+24(FP), R9
	MOVQ sub1_base+48(FP), R10
	MOVQ tw_base+72(FP), R11
	MOVQ m+96(FP), DX

	// CX = m * 8 (byte distance between out[k] and out[m+k])
	MOVQ DX, CX
	SHLQ $3, CX

loop4:
	CMPQ DX, $4
	JL loop2

	// --- 4 butterflies unrolled (32 bytes = 4 complex numbers) ---
	MOVUPS 0(R10), X0
	MOVUPS 16(R10), X7
	MOVUPS 0(R11), X1
	MOVUPS 16(R11), X8

	// Complex mult for first 2 butterflies (X0 * X1 -> X2)
	MOVSLDUP X0, X2
	MULPS X1, X2
	MOVSHDUP X0, X3
	PSHUFD $0xB1, X1, X4
	MULPS X4, X3
	ADDSUBPS X3, X2

	// Combine first 2 butterflies with sub0
	MOVUPS 0(R9), X5
	MOVAPS X5, X6
	ADDPS X2, X5
	SUBPS X2, X6
	MOVUPS X5, 0(R8)
	MOVUPS X6, 0(R8)(CX*1)

	// Complex mult for second 2 butterflies (X7 * X8 -> X9)
	MOVSLDUP X7, X9
	MULPS X8, X9
	MOVSHDUP X7, X10
	PSHUFD $0xB1, X8, X11
	MULPS X11, X10
	ADDSUBPS X10, X9

	// Combine second 2 butterflies with sub0
	MOVUPS 16(R9), X12
	MOVAPS X12, X13
	ADDPS X9, X12
	SUBPS X9, X13
	MOVUPS X12, 16(R8)
	MOVUPS X13, 16(R8)(CX*1)

	ADDQ $32, R8
	ADDQ $32, R9
	ADDQ $32, R10
	ADDQ $32, R11
	SUBQ $4, DX
	JMP loop4

loop2:
	CMPQ DX, $2
	JL loop1

	// --- 2 butterflies (16 bytes = 2 complex numbers) ---
	MOVUPS 0(R10), X0
	MOVUPS 0(R11), X1

	MOVSLDUP X0, X2
	MULPS X1, X2
	MOVSHDUP X0, X3
	PSHUFD $0xB1, X1, X4
	MULPS X4, X3
	ADDSUBPS X3, X2

	MOVUPS 0(R9), X5
	MOVAPS X5, X6
	ADDPS X2, X5
	SUBPS X2, X6
	MOVUPS X5, 0(R8)
	MOVUPS X6, 0(R8)(CX*1)

	ADDQ $16, R8
	ADDQ $16, R9
	ADDQ $16, R10
	ADDQ $16, R11
	SUBQ $2, DX
	JMP loop2

loop1:
	CMPQ DX, $1
	JL done

	// --- 1 scalar butterfly (8 bytes = 1 complex number) ---
	MOVSS 0(R10), X0   // b_re
	MOVSS 4(R10), X1   // b_im
	MOVSS 0(R11), X2   // w_re
	MOVSS 4(R11), X3   // w_im
	MOVSS 0(R9), X4    // a_re
	MOVSS 4(R9), X5    // a_im

	// t_re = b_re * w_re - b_im * w_im
	MOVSS X0, X6
	MULSS X2, X6
	MOVSS X1, X7
	MULSS X3, X7
	SUBSS X7, X6

	// t_im = b_re * w_im + b_im * w_re
	MOVSS X0, X7
	MULSS X3, X7
	MOVSS X1, X8
	MULSS X2, X8
	ADDSS X8, X7

	// out[k] = sub0 + T
	MOVSS X4, X8
	ADDSS X6, X8
	MOVSS X5, X9
	ADDSS X7, X9
	MOVSS X8, 0(R8)
	MOVSS X9, 4(R8)

	// out[m+k] = sub0 - T
	SUBSS X6, X4
	SUBSS X7, X5
	MOVSS X4, 0(R8)(CX*1)
	MOVSS X5, 4(R8)(CX*1)

done:
	RET
