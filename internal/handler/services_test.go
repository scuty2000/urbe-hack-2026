package handler_test

import (
	feedomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/fee"
	institutiondomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/institution"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/platform"
	rewarddomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/reward"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository"
)

// newFeeService and newRewardService keep every handler test's Config wiring
// identical without repeating the domain imports in each test file.

func newFeeService(db repository.Database) feedomain.Service {
	return feedomain.New(db)
}

func newRewardService(db repository.Database) rewarddomain.Service {
	return rewarddomain.New(db)
}

func newInstitutionService(db repository.Database) institutiondomain.Service {
	return institutiondomain.New(db)
}

func newPlatformService(db repository.Database) platform.Service {
	return platform.New(db)
}
