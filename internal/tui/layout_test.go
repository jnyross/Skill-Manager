package tui

import "testing"

func TestSplitPaneWidthsSumsToInput(t *testing.T) {
	for _, width := range []int{0, 1, 2, 3, 10, 39, 40, 41, 79, 80, 200} {
		list, detail := splitPaneWidths(width)
		if list < 0 || detail < 0 {
			t.Fatalf("splitPaneWidths(%d) = (%d, %d), want non-negative", width, list, detail)
		}
		if width >= 2 && list+detail+1 != width {
			t.Fatalf("splitPaneWidths(%d) = (%d, %d), want list+detail+1 == width", width, list, detail)
		}
	}
}

func TestSplitPaneWidthsBiasesTowardList(t *testing.T) {
	list, detail := splitPaneWidths(80)
	if list <= detail {
		t.Fatalf("splitPaneWidths(80) = (%d, %d), want list > detail", list, detail)
	}
}
