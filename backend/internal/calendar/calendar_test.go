package calendar

import (
	"errors"
	"testing"
	"time"
)

func TestNYSECalendar(t *testing.T) {
	tests := []struct {
		date Date
		open bool
	}{
		{Date{2024, time.July, 4}, false},
		{Date{2024, time.November, 28}, false},
		{Date{2021, time.December, 24}, false},
		{Date{2024, time.July, 3}, true},
		{Date{2024, time.November, 29}, true},
		{Date{2024, time.July, 5}, true},
	}
	for _, test := range tests {
		got, err := IsTradingDay(test.date)
		if err != nil || got != test.open {
			t.Errorf("IsTradingDay(%s) = %v, %v; want %v", test.date, got, err, test.open)
		}
	}
}

func TestTradingDayNavigation(t *testing.T) {
	got, err := AddTradingDays(Date{2024, time.July, 3}, 1)
	if err != nil || got != (Date{2024, time.July, 5}) {
		t.Fatalf("AddTradingDays = %v, %v; want 2024-07-05", got, err)
	}
	got, err = AddTradingDays(Date{2024, time.July, 5}, -1)
	if err != nil || got != (Date{2024, time.July, 3}) {
		t.Fatalf("AddTradingDays backwards = %v, %v; want 2024-07-03", got, err)
	}
}

func TestLastTradingDayOfMonth(t *testing.T) {
	got, err := LastTradingDayOfMonth(2024, time.March)
	if err != nil || got != (Date{2024, time.March, 28}) {
		t.Fatalf("LastTradingDayOfMonth = %v, %v; want 2024-03-28", got, err)
	}
}

func TestDateRangeValidation(t *testing.T) {
	_, err := IsTradingDay(Date{1989, time.December, 31})
	if !errors.Is(err, ErrOutOfRange) {
		t.Fatalf("IsTradingDay error = %v, want ErrOutOfRange", err)
	}
}

func TestDSTConversionUsesExplicitZones(t *testing.T) {
	zurich, err := time.LoadLocation("Europe/Zurich")
	if err != nil {
		t.Fatal(err)
	}
	newYork, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	// The US has switched to daylight time while Europe has not yet switched.
	zurichTime := time.Date(2024, time.March, 11, 15, 30, 0, 0, zurich)
	if got := zurichTime.In(newYork).Format("15:04"); got != "10:30" {
		t.Fatalf("New York time = %s, want 10:30 during DST mismatch", got)
	}
}

func TestEarlyClose(t *testing.T) {
	got, err := IsEarlyClose(Date{2024, time.November, 29})
	if err != nil || !got {
		t.Fatalf("IsEarlyClose = %v, %v; want true", got, err)
	}
}
