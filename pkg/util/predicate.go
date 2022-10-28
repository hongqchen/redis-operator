package util

import (
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var _ predicate.Predicate = AnnotationsOrGenerationChanged{}
var _ predicate.Predicate = PodDeleted{}

type AnnotationsOrGenerationChanged struct {
	predicate.Funcs
}

func (AnnotationsOrGenerationChanged) Update(e event.UpdateEvent) bool {
	if e.ObjectOld == nil {
		return false
	}
	if e.ObjectNew == nil {
		return false
	}

	// GenerationChanged
	if e.ObjectNew.GetGeneration() != e.ObjectOld.GetGeneration() {
		return true
	}

	// AnnotationChanged
	if !reflect.DeepEqual(e.ObjectNew.GetAnnotations(), e.ObjectOld.GetAnnotations()) {
		return true
	}

	return false
}

func (AnnotationsOrGenerationChanged) Create(e event.CreateEvent) bool {
	return false
}

type PodDeleted struct {
	predicate.Funcs
}

func (PodDeleted) Create(e event.CreateEvent) bool {
	return false
}

func (PodDeleted) Update(e event.UpdateEvent) bool {
	return false
}
