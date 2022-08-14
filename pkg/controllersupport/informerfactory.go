package controllersupport

import (
	"fmt"
	"reflect"

	"k8s.io/klog/v2"
)

func MustSyncInformer(syncResult map[reflect.Type]bool) {
	if err := CheckInformerSync(syncResult); err != nil {
		klog.Fatal(err.Error())
	}
}

func CheckInformerSync(syncResult map[reflect.Type]bool) error {
	for typ, ok := range syncResult {
		if !ok {
			return fmt.Errorf("Cache sync failed for %s, exiting", typ.String())
		}
	}

	return nil
}
