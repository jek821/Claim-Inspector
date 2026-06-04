package usage

import "testing"

func TestBillableVoyageUSD(t *testing.T) {
	if got := BillableVoyageUSD(1000, 0); got != 0 {
		t.Fatalf("under free tier: got %v want 0", got)
	}
	if got := BillableVoyageUSD(1_000_000, 199_500_000); got <= 0 {
		t.Fatalf("partial over free tier: got %v want >0", got)
	}
	if got := BillableVoyageUSD(1_000_000, 200_000_000); got <= 0 {
		t.Fatalf("after free tier exhausted: got %v want >0", got)
	}
}

func TestBillableOpenAlexUSD(t *testing.T) {
	if got := BillableOpenAlexUSD(0.05, 0); got != 0 {
		t.Fatalf("within daily credit: got %v want 0", got)
	}
	if got := BillableOpenAlexUSD(0.5, 0.9); got <= 0 {
		t.Fatalf("over daily credit: got %v want >0", got)
	}
}
