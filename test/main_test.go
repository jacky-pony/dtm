/*
 * Copyright (c) 2021 yedf. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

package test

import (
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yedf/dtm/common"
	"github.com/yedf/dtm/dtmcli"
	"github.com/yedf/dtm/dtmsvr"
	"github.com/yedf/dtm/examples"
)

func exitIf(code int) {
	if code != 0 {
		os.Exit(code)
	}
}

func TestMain(m *testing.M) {
	common.MustLoadConfig()
	dtmcli.SetCurrentDBType(common.Config.ExamplesDB.Driver)
	dtmsvr.TransProcessedTestChan = make(chan string, 1)
	dtmsvr.NowForwardDuration = 0 * time.Second
	dtmsvr.CronForwardDuration = 180 * time.Second
	common.Config.UpdateBranchSync = 1

	// 启动组件
	go dtmsvr.StartSvr()
	examples.GrpcStartup()
	app = examples.BaseAppStartup()
	app.POST(examples.BusiAPI+"/TccBSleepCancel", common.WrapHandler(func(c *gin.Context) (interface{}, error) {
		return disorderHandler(c)
	}))
	tenv := os.Getenv("TEST_STORE")
	if tenv == "boltdb" {
		config.Store.Driver = "boltdb"
	} else if tenv == "mysql" {
		config.Store.Driver = "mysql"
		config.Store.Host = "localhost"
		config.Store.Port = 3306
		config.Store.User = "root"
		config.Store.Password = ""
	} else {
		config.Store.Driver = "redis"
		config.Store.Host = "localhost"
		config.Store.Port = 6379
	}
	dtmsvr.PopulateDB(false)
	examples.PopulateDB(false)
	exitIf(m.Run())

}
