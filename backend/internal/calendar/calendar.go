package calendar

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrOutOfRange   = errors.New("date outside supported NYSE calendar range")
	ErrNoTradingDay = errors.New("no trading day found in supported range")
)

const (
	FirstSupportedYear = 1990
	LastSupportedYear  = 2035
)

type Date struct {
	Year  int
	Month time.Month
	Day   int
}

func FromTime(t time.Time) Date {
	return Date{Year: t.Year(), Month: t.Month(), Day: t.Day()}
}

func (d Date) Time() time.Time {
	return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC)
}

func (d Date) String() string {
	return d.Time().Format("2006-01-02")
}

func validate(d Date) error {
	t := d.Time()
	if t.Year() != d.Year || t.Month() != d.Month || t.Day() != d.Day {
		return fmt.Errorf("invalid civil date %v", d)
	}
	if d.Year < FirstSupportedYear || d.Year > LastSupportedYear {
		return fmt.Errorf("%w: %d", ErrOutOfRange, d.Year)
	}
	return nil
}

func IsTradingDay(d Date) (bool, error) {
	if err := validate(d); err != nil {
		return false, err
	}
	t := d.Time()
	if t.Weekday() == time.Saturday || t.Weekday() == time.Sunday || isHoliday(t) {
		return false, nil
	}
	return true, nil
}

func NextTradingDay(d Date) (Date, error) {
	return moveTradingDays(d, 1)
}

func PreviousTradingDay(d Date) (Date, error) {
	return moveTradingDays(d, -1)
}

func AddTradingDays(d Date, days int) (Date, error) {
	if err := validate(d); err != nil {
		return Date{}, err
	}
	if days == 0 {
		return d, nil
	}
	return moveTradingDays(d, days)
}

func moveTradingDays(d Date, direction int) (Date, error) {
	if err := validate(d); err != nil {
		return Date{}, err
	}
	step := 1
	if direction < 0 {
		step = -1
	}
	remaining := direction
	for remaining != 0 {
		t := d.Time().AddDate(0, 0, step)
		d = FromTime(t)
		if err := validate(d); err != nil {
			return Date{}, err
		}
		open, err := IsTradingDay(d)
		if err != nil {
			return Date{}, err
		}
		if open {
			remaining -= step
		}
	}
	return d, nil
}

func LastTradingDayOfMonth(year int, month time.Month) (Date, error) {
	if year < FirstSupportedYear || year > LastSupportedYear {
		return Date{}, fmt.Errorf("%w: %d", ErrOutOfRange, year)
	}
	if month < time.January || month > time.December {
		return Date{}, fmt.Errorf("invalid month %d", month)
	}
	d := FromTime(time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC))
	for {
		open, err := IsTradingDay(d)
		if err != nil {
			return Date{}, err
		}
		if open {
			return d, nil
		}
		d = FromTime(d.Time().AddDate(0, 0, -1))
	}
}

func TradingDaysBetween(from, to Date) ([]Date, error) {
	if err := validate(from); err != nil {
		return nil, err
	}
	if err := validate(to); err != nil {
		return nil, err
	}
	if to.Time().Before(from.Time()) {
		return nil, fmt.Errorf("end date %s is before start date %s", to, from)
	}
	var dates []Date
	for t := from.Time(); !t.After(to.Time()); t = t.AddDate(0, 0, 1) {
		d := FromTime(t)
		open, err := IsTradingDay(d)
		if err != nil {
			return nil, err
		}
		if open {
			dates = append(dates, d)
		}
	}
	return dates, nil
}

func IsEarlyClose(d Date) (bool, error) {
	if err := validate(d); err != nil {
		return false, err
	}
	open, err := IsTradingDay(d)
	if err != nil || !open {
		return false, err
	}
	t := d.Time()
	if t.Month() == time.July && t.Day() == 3 && t.Weekday() != time.Saturday && t.Weekday() != time.Sunday {
		return true, nil
	}
	if t.Month() == time.December && t.Day() == 24 && t.Weekday() != time.Saturday && t.Weekday() != time.Sunday {
		return true, nil
	}
	if t.Month() == time.November && t.Weekday() == time.Friday && t.Day() >= 23 && t.Day() <= 29 {
		return true, nil
	}
	return false, nil
}

func isHoliday(t time.Time) bool {
	y := t.Year()
	if isObservedFixedHoliday(t, time.January, 1) ||
		isObservedFixedHoliday(t, time.June, 19) && y >= 2022 ||
		isObservedFixedHoliday(t, time.July, 4) ||
		isObservedFixedHoliday(t, time.December, 25) {
		return true
	}
	if t.Equal(nthWeekday(y, time.January, time.Monday, 3)) ||
		t.Equal(nthWeekday(y, time.February, time.Monday, 3)) ||
		t.Equal(lastWeekday(y, time.May, time.Monday)) ||
		t.Equal(nthWeekday(y, time.September, time.Monday, 1)) ||
		t.Equal(nthWeekday(y, time.November, time.Thursday, 4)) ||
		t.Equal(easterSunday(y).AddDate(0, 0, -2)) {
		return true
	}
	if y >= 1998 && t.Equal(nthWeekday(y, time.October, time.Monday, 2)) {
		return true
	}
	return false
}

func easterSunday(year int) time.Time {
	a := year % 19
	b, c := year/100, year%100
	d, e := b/4, b%4
	f := (b + 8) / 25
	g := (b - f + 1) / 3
	h := (19*a + b - d - g + 15) % 30
	i, k := c/4, c%4
	l := (32 + 2*e + 2*i - h - k) % 7
	m := (a + 11*h + 22*l) / 451
	month := (h + l - 7*m + 114) / 31
	day := (h+l-7*m+114)%31 + 1
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}

func isObservedFixedHoliday(t time.Time, month time.Month, day int) bool {
	y := t.Year()
	holiday := time.Date(y, month, day, 0, 0, 0, 0, time.UTC)
	switch holiday.Weekday() {
	case time.Saturday:
		holiday = holiday.AddDate(0, 0, -1)
	case time.Sunday:
		holiday = holiday.AddDate(0, 0, 1)
	}
	return t.Equal(holiday)
}

func nthWeekday(year int, month time.Month, weekday time.Weekday, n int) time.Time {
	t := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	offset := (int(weekday) - int(t.Weekday()) + 7) % 7
	return t.AddDate(0, 0, offset+7*(n-1))
}

func lastWeekday(year int, month time.Month, weekday time.Weekday) time.Time {
	t := time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC)
	offset := (int(t.Weekday()) - int(weekday) + 7) % 7
	return t.AddDate(0, 0, -offset)
}
