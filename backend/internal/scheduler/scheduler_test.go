package scheduler

import (
	"testing"
	"time"
)

func TestDailyScheduleTimeRespectsMarketCloseHolidaysAndDST(t *testing.T) {
	zurich, err := time.LoadLocation("Europe/Zurich")
	if err != nil {
		t.Fatal(err)
	}
	newYork, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		now  time.Time
		due  bool
		want string
	}{
		{
			name: "before close buffer",
			now:  time.Date(2024, time.July, 8, 16, 29, 0, 0, newYork),
			due:  false,
		},
		{
			name: "summer after close",
			now:  time.Date(2024, time.July, 8, 16, 30, 0, 0, newYork),
			due:  true,
			want: "2024-07-08T20:30:00Z",
		},
		{
			name: "winter after close",
			now:  time.Date(2024, time.January, 8, 16, 30, 0, 0, newYork),
			due:  true,
			want: "2024-01-08T21:30:00Z",
		},
		{
			name: "NYSE holiday",
			now:  time.Date(2024, time.July, 4, 16, 30, 0, 0, newYork),
			due:  false,
		},
		{
			name: "weekend",
			now:  time.Date(2024, time.July, 6, 16, 30, 0, 0, newYork),
			due:  false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, due, err := DailyScheduleTime(test.now, zurich, newYork)
			if err != nil {
				t.Fatal(err)
			}
			if due != test.due {
				t.Fatalf("due = %v, want %v", due, test.due)
			}
			if due && got.Format(time.RFC3339) != test.want {
				t.Fatalf("scheduled at %s, want %s", got.Format(time.RFC3339), test.want)
			}
		})
	}
}
