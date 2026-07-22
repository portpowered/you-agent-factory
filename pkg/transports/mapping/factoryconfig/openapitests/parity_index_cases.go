package openapitests

func baselineAcceptParityCases() []ParityCase {
	cases := make([]ParityCase, 0, 10)
	cases = append(cases, baselineAcceptParityCasesWorkstationLayoutOrchestrator()...)
	cases = append(cases, baselineAcceptParityCasesWorkerResourceGuard()...)
	return cases
}

func baselineAcceptParityCasesWorkstationLayoutOrchestrator() []ParityCase {
	return []ParityCase{
		{
			ID:            "accept-canonical-camel-case-workstation",
			Shape:         shapeWorkstation,
			Category:      categoryShapeMapping,
			SourceTest:    "openapi_factory_test.go:TestFactoryConfigFromOpenAPIJSON_MapsCanonicalCamelCaseWorkstationSchema",
			Fixture:       "accept/canonical-camel-case-workstation.json",
			Description:   "canonical camelCase workstation schema maps through API and config-loader boundaries",
			APIOutcome:    outcomeAccept,
			LoaderOutcome: outcomeAccept,
		},
		{
			ID:            "accept-graphable-entity-ids",
			Shape:         shapeWorkstation,
			Category:      categoryShapeMapping,
			SourceTest:    "openapi_factory_test.go:TestFactoryConfigFromOpenAPIJSON_MapsOptionalGraphableEntityIDs",
			Fixture:       "accept/graphable-entity-ids.json",
			Description:   "optional graphable entity ids roundtrip through generated and internal config",
			APIOutcome:    outcomeAccept,
			LoaderOutcome: outcomeAccept,
		},
		{
			ID:            "accept-portable-layout-contract",
			Shape:         shapeLayout,
			Category:      categoryLayoutContract,
			SourceTest:    "openapi_factory_test.go:TestFactoryConfigFromOpenAPIJSON_MapsPortableLayoutContract",
			Fixture:       "accept/portable-layout-contract.json",
			Description:   "portable layout contract maps through API and config-loader boundaries",
			APIOutcome:    outcomeAccept,
			LoaderOutcome: outcomeAccept,
		},
		{
			ID:            "accept-javascript-orchestrator",
			Shape:         shapeOrchestrator,
			Category:      categoryShapeMapping,
			SourceTest:    "openapi_factory_orchestrator_test.go:TestFactoryConfigFromOpenAPIJSON_RoundTripsJavaScriptOrchestratorFactory",
			Fixture:       "accept/javascript-orchestrator.json",
			Description:   "explicit JavaScript orchestrator factory roundtrips through API and config-loader boundaries",
			APIOutcome:    outcomeAccept,
			LoaderOutcome: outcomeAccept,
		},
		{
			ID:            "accept-model-invoke-operation",
			Shape:         shapeWorkstation,
			Category:      categoryShapeMapping,
			SourceTest:    "openapi_factory_model_operation_test.go:TestFactoryConfigFromOpenAPIJSON_MapsModelInvokeOperation",
			Fixture:       "accept/model-invoke-operation.json",
			Description:   "model invoke workstation operation bindings map through API and config-loader boundaries",
			APIOutcome:    outcomeAccept,
			LoaderOutcome: outcomeAccept,
		},
	}
}

func baselineAcceptParityCasesWorkerResourceGuard() []ParityCase {
	return []ParityCase{
		{
			ID:            "accept-hosted-linear-worker",
			Shape:         shapeWorker,
			Category:      categoryShapeMapping,
			SourceTest:    "openapi_factory_hosted_worker_test.go:TestGeneratedFactoryFromOpenAPIJSON_DecodesHostedLinearWorker",
			Fixture:       "accept/hosted-linear-worker.json",
			Description:   "hosted linear worker decodes through generated and internal config boundaries",
			APIOutcome:    outcomeAccept,
			LoaderOutcome: outcomeAccept,
		},
		{
			ID:            "accept-inference-worker-taxonomy",
			Shape:         shapeWorker,
			Category:      categoryTaxonomyEnum,
			SourceTest:    "openapi_factory_worker_taxonomy_test.go:TestFactoryConfigFromOpenAPIJSON_AcceptsNewWorkerTaxonomyAndProjectsLegacyRuntimeTypes",
			Fixture:       "accept/inference-worker-taxonomy.json",
			Description:   "INFERENCE_WORKER taxonomy enum maps through API and config-loader boundaries",
			APIOutcome:    outcomeAccept,
			LoaderOutcome: outcomeAccept,
		},
		{
			ID:            "accept-inference-run-workstation-taxonomy",
			Shape:         shapeWorkstation,
			Category:      categoryTaxonomyEnum,
			SourceTest:    "openapi_factory_workstation_taxonomy_test.go:TestFactoryConfigFromOpenAPIJSON_AcceptsNewWorkstationTaxonomyAndProjectsLegacyRuntimeTypes",
			Fixture:       "accept/inference-run-workstation-taxonomy.json",
			Description:   "INFERENCE_RUN workstation taxonomy enum maps through API and config-loader boundaries",
			APIOutcome:    outcomeAccept,
			LoaderOutcome: outcomeAccept,
		},
		{
			ID:            "accept-typed-model-resource",
			Shape:         shapeResource,
			Category:      categoryShapeMapping,
			SourceTest:    "openapi_factory_model_operation_test.go:TestFactoryConfigFromOpenAPIJSON_MapsTypedModelResources",
			Fixture:       "accept/typed-model-resource.json",
			Description:   "typed MODEL resource metadata maps through API and config-loader boundaries",
			APIOutcome:    outcomeAccept,
			LoaderOutcome: outcomeAccept,
		},
		{
			ID:            "accept-same-name-input-guard",
			Shape:         shapeGuard,
			Category:      categoryGuardUnion,
			SourceTest:    "openapi_factory_test.go:TestGeneratedFactoryFromOpenAPIJSON_DecodesSameNameInputGuard",
			Fixture:       "accept/same-name-input-guard.json",
			Description:   "SAME_NAME input guard union decodes through generated and internal config boundaries",
			APIOutcome:    outcomeAccept,
			LoaderOutcome: outcomeAccept,
		},
	}
}

func baselineRejectParityCases() []ParityCase {
	cases := make([]ParityCase, 0, 8)
	cases = append(cases, baselineRejectParityCasesWorkerWorkstation()...)
	cases = append(cases, baselineRejectParityCasesLayoutGuardOrchestratorResource()...)
	return cases
}

func baselineRejectParityCasesWorkerWorkstation() []ParityCase {
	return []ParityCase{
		{
			ID:                    "reject-miscased-worker-type",
			Shape:                 shapeWorker,
			Category:              categoryBoundaryEnum,
			SourceTest:            "openapi_factory_boundary_enum_test.go:TestGeneratedFactoryFromOpenAPIJSON_RejectsMisCasedEnumValuesAtBoundary",
			Fixture:               "reject/miscased-worker-type.json",
			Description:           "mis-cased worker type enum fails at generated-schema boundary",
			APIOutcome:            outcomeReject,
			LoaderOutcome:         outcomeReject,
			ExpectedErrorPath:     "workers[0].type",
			ExpectedErrorCategory: categoryBoundaryEnum,
			ErrorFragments: []string{
				"decode factory generated-schema boundary",
				`unsupported value "model_worker"`,
			},
		},
		{
			ID:                    "reject-unknown-worker-model-provider",
			Shape:                 shapeWorker,
			Category:              categoryBoundaryEnum,
			SourceTest:            "openapi_factory_model_provider_roundtrip_test.go:TestGeneratedFactoryFromOpenAPIJSON_RejectsUnknownWorkerModelProviderAtBoundary",
			Fixture:               "reject/unknown-worker-model-provider.json",
			Description:           "unknown worker model provider fails at generated-schema boundary",
			APIOutcome:            outcomeReject,
			LoaderOutcome:         outcomeReject,
			ExpectedErrorPath:     "workers[0].modelProvider",
			ExpectedErrorCategory: categoryBoundaryEnum,
			ErrorFragments: []string{
				"decode factory generated-schema boundary",
				`unsupported value "MYSTERY-PROVIDER"`,
			},
		},
		{
			ID:                    "reject-miscased-workstation-type",
			Shape:                 shapeWorkstation,
			Category:              categoryBoundaryEnum,
			SourceTest:            "openapi_factory_boundary_enum_test.go:TestGeneratedFactoryFromOpenAPIJSON_RejectsMisCasedEnumValuesAtBoundary",
			Fixture:               "reject/miscased-workstation-type.json",
			Description:           "mis-cased workstation type enum fails at generated-schema boundary",
			APIOutcome:            outcomeReject,
			LoaderOutcome:         outcomeReject,
			ExpectedErrorPath:     "workstations[0].type",
			ExpectedErrorCategory: categoryBoundaryEnum,
			ErrorFragments: []string{
				"decode factory generated-schema boundary",
				`unsupported value "logical_move"`,
			},
		},
		{
			ID:                    "reject-retired-fan-in-join",
			Shape:                 shapeWorkstation,
			Category:              categoryRetiredBoundary,
			SourceTest:            "openapi_factory_retired_boundary_test.go:TestGeneratedFactoryFromOpenAPIJSON_RejectsRetiredFanInFieldAtBoundary",
			Fixture:               "reject/retired-fan-in-join.json",
			Description:           "retired workstation join field fails at generated-schema boundary",
			APIOutcome:            outcomeReject,
			LoaderOutcome:         outcomeReject,
			ExpectedErrorPath:     "workstations[0].join",
			ExpectedErrorCategory: categoryRetiredBoundary,
			ErrorFragments: []string{
				"decode factory generated-schema boundary",
				"workstations[0].join is not supported",
			},
		},
	}
}

func baselineRejectParityCasesLayoutGuardOrchestratorResource() []ParityCase {
	return []ParityCase{
		{
			ID:                    "reject-malformed-layout-missing-schema-version",
			Shape:                 shapeLayout,
			Category:              categoryLayoutContract,
			SourceTest:            "openapi_factory_test.go:TestFactoryConfigFromOpenAPIJSON_RejectsMalformedPortableLayoutContract",
			Fixture:               "reject/malformed-layout-missing-schema-version.json",
			Description:           "layout payload missing schemaVersion fails at generated-schema and config-loader boundaries",
			APIOutcome:            outcomeReject,
			LoaderOutcome:         outcomeReject,
			ExpectedErrorPath:     "layout.schemaVersion",
			ExpectedErrorCategory: categoryLayoutContract,
			ErrorFragments: []string{
				"decode factory generated-schema boundary",
				"layout.schemaVersion is required",
			},
		},
		{
			ID:                    "reject-inference-throttle-on-workstation-guard",
			Shape:                 shapeGuard,
			Category:              categoryGuardUnion,
			SourceTest:            "openapi_factory_test.go:TestGeneratedFactoryFromOpenAPIJSON_RejectsInferenceThrottleGuardOnWorkstation",
			Fixture:               "reject/inference-throttle-on-workstation-guard.json",
			Description:           "root-only inference throttle guard fails on workstation guard union",
			APIOutcome:            outcomeReject,
			LoaderOutcome:         outcomeReject,
			ExpectedErrorPath:     "workstations[0].guards[0].type",
			ExpectedErrorCategory: categoryGuardUnion,
			ErrorFragments: []string{
				"decode factory generated-schema boundary",
				"workstations[0].guards[0].type",
			},
		},
		{
			ID:                    "reject-miscased-orchestrator-kind",
			Shape:                 shapeOrchestrator,
			Category:              categoryBoundaryEnum,
			SourceTest:            "openapi_factory_boundary_enum_test.go:TestGeneratedFactoryFromOpenAPIJSON_RejectsMisCasedEnumValuesAtBoundary",
			Fixture:               "reject/miscased-orchestrator-kind.json",
			Description:           "mis-cased orchestrator kind enum fails at generated-schema boundary",
			APIOutcome:            outcomeReject,
			LoaderOutcome:         outcomeReject,
			ExpectedErrorPath:     "orchestrator.kind",
			ExpectedErrorCategory: categoryBoundaryEnum,
			ErrorFragments: []string{
				"decode factory generated-schema boundary",
				`unsupported value "javascript"`,
			},
		},
		{
			ID:                    "reject-miscased-resource-type",
			Shape:                 shapeResource,
			Category:              categoryBoundaryEnum,
			SourceTest:            "openapi_factory_boundary_enum_test.go:TestGeneratedFactoryFromOpenAPIJSON_RejectsMisCasedEnumValuesAtBoundary",
			Fixture:               "reject/miscased-resource-type.json",
			Description:           "mis-cased resource type enum fails at generated-schema boundary",
			APIOutcome:            outcomeReject,
			LoaderOutcome:         outcomeReject,
			ExpectedErrorPath:     "resources[0].type",
			ExpectedErrorCategory: categoryBoundaryEnum,
			ErrorFragments: []string{
				"decode factory generated-schema boundary",
				`unsupported value "model"`,
			},
		},
	}
}
