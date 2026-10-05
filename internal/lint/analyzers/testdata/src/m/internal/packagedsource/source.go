package packagedsource

var raw = `{"name":" @you/raw "}` // want `packaged-factory-source:.*@you/raw.*packages/packaged-factories/factories`
var duplicate = `{"name":"@you/raw"}`
var yaml = "name: '@you/yaml'\n" // want `packaged-factory-source:.*@you/yaml`
var malformed = `{"name":`
var scalar = "@you/scalar"
var empty = `{"name":" "}`
var other = `{"name":"@You/case"}`
var customer = `{"name":"customer"}`
