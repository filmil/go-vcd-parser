package db

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// BenchmarkCacheSize measures what SQLite's page cache is worth during a
// bulk load, in time and in resident memory. A load writes each page once
// and never reads it back, which is why the answer is "nothing", and why
// bulkParams asks for a small cache.
//
// Resident size is process-wide, so the rss_MB metric is only meaningful
// when this benchmark runs on its own:
//
//	bazel test //db:db_test --test_output=all \
//	    --test_arg=-test.bench=CacheSize --test_arg=-test.run=XXX
func BenchmarkCacheSize(b *testing.B) {
	saved := bulkParams
	b.Cleanup(func() { bulkParams = saved })

	for _, kib := range []int{2000, 8000, 20000, 200000} {
		b.Run(strconv.Itoa(kib)+"KiB", func(b *testing.B) {
			bulkParams = fmt.Sprintf(
				"_journal_mode=OFF&_synchronous=OFF&_cache_size=-%d", kib)
			ctx := context.Background()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				loadRows(b, ctx, i)
			}
			b.StopTimer()
			if mb, ok := residentMB(); ok {
				b.ReportMetric(mb, "rss_MB")
			}
		})
	}
}

const cacheBenchRows = 200_000

func loadRows(b *testing.B, ctx context.Context, i int) {
	b.Helper()
	name := filepath.Join(b.TempDir(), fmt.Sprintf("cache.%d.db", i))
	dbf, err := OpenBulk(ctx, name)
	if err != nil {
		b.Fatalf("could not open: %v", err)
	}
	defer dbf.Close()
	tx, err := dbf.Begin()
	if err != nil {
		b.Fatalf("could not begin: %v", err)
	}
	ins, err := Prepare(ctx, tx)
	if err != nil {
		b.Fatalf("could not prepare: %v", err)
	}
	for j := 0; j < cacheBenchRows; j++ {
		if err := ins.AddValue(ctx, uint64(j)*500,
			fmt.Sprintf("c%d", j%400), fmt.Sprintf("b%b", j%4096)); err != nil {
			b.Fatalf("could not add: %v", err)
		}
	}
	if err := ins.Close(); err != nil {
		b.Fatalf("could not close: %v", err)
	}
	if err := tx.Commit(); err != nil {
		b.Fatalf("could not commit: %v", err)
	}
	if err := FinishBulk(ctx, dbf); err != nil {
		b.Fatalf("could not finish: %v", err)
	}
}

// residentMB reads the current resident set size. SQLite's page cache is
// allocated by C code, so Go's own MemStats cannot see it.
func residentMB() (float64, bool) {
	buf, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return 0, false // not Linux
	}
	var total, resident int64
	if _, err := fmt.Sscan(string(buf), &total, &resident); err != nil {
		return 0, false
	}
	return float64(resident*int64(os.Getpagesize())) / (1 << 20), true
}
