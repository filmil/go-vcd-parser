/*
 * Bridge between libfst's callback-driven block reader and cgo.
 *
 * SPDX-License-Identifier: Apache-2.0
 *
 * cgo cannot pass a Go function to C directly, and a file that uses
 * //export may not define C functions in its preamble. So the call to
 * fstReaderIterBlocks2 lives here, in a real translation unit, and names
 * the two exported Go callbacks.
 */
#ifndef GO_VCD_PARSER_FST_BRIDGE_H
#define GO_VCD_PARSER_FST_BRIDGE_H

#include "fstapi.h"

/* Runs the block reader, reporting every value change to the Go callbacks.
 * ud is a runtime/cgo.Handle identifying the reader that owns this run. */
int goFstIterBlocks2(fstReaderContext *ctx, void *ud);

#endif
