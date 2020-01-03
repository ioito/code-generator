package models

import (
	"reflect"

	"k8s.io/gengo/types"

	"yunion.io/x/onecloud/pkg/appsrv"
	"yunion.io/x/onecloud/pkg/cloudcommon/db"
	computesvc "yunion.io/x/onecloud/pkg/compute/service"
	imagesvc "yunion.io/x/onecloud/pkg/image/service"
	identitysvc "yunion.io/x/onecloud/pkg/keystone/service"
)

func init() {
	for _, f := range []func(*appsrv.Application){
		computesvc.InitHandlers,
		imagesvc.InitHandlers,
		identitysvc.InitHandlers,
	} {
		app := appsrv.NewApplication("", 1, false)
		f(app)
		// hack: delete duplicate tasks model register
		for _, key := range []string{
			"task", "subtask", "taskobject", "user", "tenant",
			"shared_resource", "quota_usage", "quota_pending_usage",
			"event", "metadata"} {
			delete(db.GlobalModelManagerTables(), key)
		}
	}
	for _, man := range db.GlobalModelManagerTables() {
		RegisterModelManager(man)
	}
}

var globalManagers map[string]db.IModelManager

func GlobalManagers() map[string]db.IModelManager {
	return globalManagers
}

func RegisterModelManager(man db.IModelManager) {
	if globalManagers == nil {
		globalManagers = make(map[string]db.IModelManager)
	}
	manType := reflect.TypeOf(man)
	if manType.Kind() == reflect.Ptr {
		manType = manType.Elem()
	}
	globalManagers[manType.Name()] = man
}

func GetModelManager(typeName string) db.IModelManager {
	return globalManagers[typeName]
}

func GetModelManagerByType(t *types.Type) db.IModelManager {
	if t == nil {
		return nil
	}
	return GetModelManager(t.Name.Name)
}
