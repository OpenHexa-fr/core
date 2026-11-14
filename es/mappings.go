package es

// Types de mapping Elasticsearch réutilisables par les backends.
//
// Les mappings des domaines métier (DVF, DPE, stations, ...) doivent être
// construits en combinant ces constructeurs plutôt qu'en laissant Elasticsearch
// inférer les types. Chaque appel retourne une nouvelle map : un mapping peut
// ainsi être modifié sans affecter les autres.

func fieldType(name string) map[string]any { return map[string]any{"type": name} }

// Keyword retourne le mapping d'un champ `keyword`.
func Keyword() map[string]any { return fieldType("keyword") }

// Text retourne le mapping d'un champ `text`.
func Text() map[string]any { return fieldType("text") }

// Date retourne le mapping d'un champ `date`.
func Date() map[string]any { return fieldType("date") }

// GeoPoint retourne le mapping d'un champ `geo_point`.
func GeoPoint() map[string]any { return fieldType("geo_point") }

// Integer retourne le mapping d'un champ `integer`.
func Integer() map[string]any { return fieldType("integer") }

// Float retourne le mapping d'un champ `float`.
func Float() map[string]any { return fieldType("float") }

// Boolean retourne le mapping d'un champ `boolean`.
func Boolean() map[string]any { return fieldType("boolean") }
