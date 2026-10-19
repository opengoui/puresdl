//go:build darwin || linux || windows

#include "textflag.h"

// trampoline is entered from runtime.asmcgocall on the system stack with the
// AAPCS64 calling convention (also used by Windows on Arm for non-variadic
// functions) and R0 = *callFrame (see call_trampoline.go and
// call_regclass.go for the layout). It keeps the frame pointer in the
// callee-saved R19, loads the eight integer and eight floating-point argument
// registers, calls fn and stores R0 and D0 back into the frame. Unused
// registers carry zeros, which a C function ignores. It does not touch R18,
// the platform register (the TEB pointer on Windows).
TEXT trampoline<>(SB), NOSPLIT|NOFRAME, $0
	SUB	$16, RSP
	MOVD	R30, 0(RSP)
	MOVD	R19, 8(RSP)
	MOVD	R0, R19

	FMOVD	72(R19), F0
	FMOVD	80(R19), F1
	FMOVD	88(R19), F2
	FMOVD	96(R19), F3
	FMOVD	104(R19), F4
	FMOVD	112(R19), F5
	FMOVD	120(R19), F6
	FMOVD	128(R19), F7

	MOVD	8(R19), R0
	MOVD	16(R19), R1
	MOVD	24(R19), R2
	MOVD	32(R19), R3
	MOVD	40(R19), R4
	MOVD	48(R19), R5
	MOVD	56(R19), R6
	MOVD	64(R19), R7

	MOVD	0(R19), R16
	CALL	(R16)

	MOVD	R0, 136(R19)
	FMOVD	F0, 144(R19)

	MOVD	8(RSP), R19
	MOVD	0(RSP), R30
	ADD	$16, RSP
	RET

GLOBL ·trampolineABI0(SB), NOPTR|RODATA, $8
DATA ·trampolineABI0(SB)/8, $trampoline<>(SB)
