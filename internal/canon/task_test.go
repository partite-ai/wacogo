package canon

import "testing"

func TestTask_End_DrainsReleases(t *testing.T) {
	var task Task
	called := 0
	task.AddRelease(func() { called++ })
	task.AddRelease(func() { called++ })

	if err := task.End(); err != nil {
		t.Fatalf("End returned %v, want nil", err)
	}
	if called != 2 {
		t.Fatalf("releases fired %d times, want 2", called)
	}
}

func TestTask_End_TrapsOnOutstandingBorrows(t *testing.T) {
	var task Task
	task.BorrowIssued()
	task.BorrowIssued()
	err := task.End()
	if err == nil {
		t.Fatal("End returned nil, want outstanding-borrows error")
	}
	want := "call returned with 2 outstanding borrows"
	if err.Error() != want {
		t.Fatalf("End err = %q, want %q", err.Error(), want)
	}
}

func TestTask_BorrowIssued_BorrowDropped_Pair(t *testing.T) {
	var task Task
	task.BorrowIssued()
	task.BorrowIssued()
	task.BorrowDropped()
	if err := task.End(); err == nil {
		t.Fatal("End returned nil with 1 outstanding borrow, want error")
	}
}

func TestTask_End_DrainsAndReportsBorrows(t *testing.T) {
	var task Task
	called := 0
	task.AddRelease(func() { called++ })
	task.BorrowIssued()
	err := task.End()
	if err == nil {
		t.Fatal("expected outstanding-borrow error")
	}
	if called != 1 {
		t.Fatalf("release fired %d times, want 1 (releases drain even when borrows outstanding)", called)
	}
}
