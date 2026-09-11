/* SPDX-License-Identifier: Apache-2.0 */
#include "bridge.h"

/* Defined in Go, in fst.go, via //export. */
extern void goFstValueChange(void *ud, uint64_t tim, fstHandle facidx,
                             const unsigned char *value);
extern void goFstValueChangeVarlen(void *ud, uint64_t tim, fstHandle facidx,
                                   const unsigned char *value, uint32_t len);

int goFstIterBlocks2(fstReaderContext *ctx, void *ud)
{
    /* The trailing NULL is the optional FILE* that libfst would write a VCD
     * to. We consume the changes through the callbacks instead. */
    return fstReaderIterBlocks2(ctx, goFstValueChange, goFstValueChangeVarlen,
                                ud, NULL);
}
