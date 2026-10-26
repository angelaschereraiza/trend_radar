package validation

import (
	"errors"
	"fmt"
	"time"
)

type Role string

const (
	Development Role = "development"
	WalkForward Role = "walk_forward"
	Holdout     Role = "holdout"
	Forward     Role = "forward"
)

type Window struct {
	TrainStart time.Time
	TrainEnd   time.Time
	TestStart  time.Time
	TestEnd    time.Time
}

type DatasetSplit struct {
	DevelopmentStart time.Time
	DevelopmentEnd   time.Time
	HoldoutStart     time.Time
	HoldoutEnd       time.Time
	ForwardStart     time.Time
}

func DefaultDatasetSplit() DatasetSplit {
	return DatasetSplit{
		DevelopmentStart: time.Date(2004, time.January, 1, 0, 0, 0, 0, time.UTC),
		DevelopmentEnd:   time.Date(2017, time.December, 31, 0, 0, 0, 0, time.UTC),
		HoldoutStart:     time.Date(2018, time.January, 1, 0, 0, 0, 0, time.UTC),
		HoldoutEnd:       time.Date(2025, time.December, 31, 0, 0, 0, 0, time.UTC),
		ForwardStart:     time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
	}
}

func (s DatasetSplit) Validate() error {
	if s.DevelopmentStart.IsZero() || s.DevelopmentEnd.IsZero() ||
		s.HoldoutStart.IsZero() || s.HoldoutEnd.IsZero() || s.ForwardStart.IsZero() {
		return errors.New("all dataset split boundaries are required")
	}
	if s.DevelopmentEnd.Before(s.DevelopmentStart) || s.HoldoutEnd.Before(s.HoldoutStart) {
		return errors.New("dataset split periods must be chronological")
	}
	if !s.HoldoutStart.After(s.DevelopmentEnd) || !s.ForwardStart.After(s.HoldoutEnd) {
		return errors.New("development, holdout, and forward periods must not overlap")
	}
	return nil
}

func (s DatasetSplit) RoleForDate(date time.Time) (Role, error) {
	if err := s.Validate(); err != nil {
		return "", err
	}
	if date.IsZero() {
		return "", errors.New("date is required")
	}
	switch {
	case !date.Before(s.DevelopmentStart) && !date.After(s.DevelopmentEnd):
		return Development, nil
	case !date.Before(s.HoldoutStart) && !date.After(s.HoldoutEnd):
		return Holdout, nil
	case !date.After(s.ForwardStart):
		return Forward, nil
	default:
		return "", fmt.Errorf("date %s is outside configured dataset periods", date.Format("2006-01-02"))
	}
}

func RequireHoldoutConfirmation(role Role, confirmed bool) error {
	if role == Holdout && !confirmed {
		return errors.New("holdout runs require explicit confirmation")
	}
	if role != Development && role != WalkForward && role != Holdout && role != Forward {
		return fmt.Errorf("unsupported dataset role %q", role)
	}
	return nil
}

func WalkForwardWindows(start, end time.Time, trainingYears, testYears, stepYears int) ([]Window, error) {
	if start.IsZero() || end.IsZero() || end.Before(start) {
		return nil, errors.New("valid chronological start and end dates are required")
	}
	if trainingYears < 1 || testYears < 1 || stepYears < 1 {
		return nil, errors.New("training, test and step years must be positive")
	}
	if stepYears < testYears {
		return nil, errors.New("walk-forward step must be at least as long as the test period")
	}
	var windows []Window
	trainStart := start
	for {
		trainEnd := trainStart.AddDate(trainingYears, 0, 0).AddDate(0, 0, -1)
		testStart := trainEnd.AddDate(0, 0, 1)
		testEnd := testStart.AddDate(testYears, 0, 0).AddDate(0, 0, -1)
		if testEnd.After(end) {
			break
		}
		if !testStart.After(trainEnd) {
			return nil, fmt.Errorf("walk-forward window overlaps train and test")
		}
		windows = append(windows, Window{
			TrainStart: trainStart, TrainEnd: trainEnd,
			TestStart: testStart, TestEnd: testEnd,
		})
		trainStart = trainStart.AddDate(stepYears, 0, 0)
	}
	return windows, nil
}

func DatasetRoleForDate(date, developmentStart, developmentEnd, holdoutStart, holdoutEnd time.Time) (Role, error) {
	if date.IsZero() {
		return "", errors.New("date is required")
	}
	switch {
	case !date.Before(developmentStart) && !date.After(developmentEnd):
		return Development, nil
	case !date.Before(holdoutStart) && !date.After(holdoutEnd):
		return Holdout, nil
	default:
		return "", fmt.Errorf("date %s is outside configured development and holdout periods", date.Format("2006-01-02"))
	}
}
