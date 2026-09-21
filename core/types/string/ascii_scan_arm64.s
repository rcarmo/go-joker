//go:build arm64 && !purego

#include "textflag.h"

TEXT ·scanASCIINEON(SB), NOSPLIT, $0-17
 MOVD s_base+0(FP), R0
 MOVD s_len+8(FP), R1
 CMP $16, R1
 BLT tail
loop:
 VLD1 (R0), [V0.B16]
 VUSHR $7, V0.B16, V1.B16
 VUADDLV V1.B16, V1
 VMOV V1.H[0], R2
 CBNZ R2, no
 ADD $16, R0
 SUB $16, R1
 CMP $16, R1
 BGE loop
tail:
 CBZ R1, yes
 MOVBU (R0), R2
 TBNZ $7, R2, no
 ADD $1, R0
 SUB $1, R1
 B tail
no:
 MOVD $0, R2
 MOVB R2, ret+16(FP)
 RET
yes:
 MOVD $1, R2
 MOVB R2, ret+16(FP)
 RET
