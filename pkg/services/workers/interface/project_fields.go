package mockworkers

func topologyFieldRecords() []FieldRecord {
	fields := make([]FieldRecord, 0, 26)
	fields = append(fields, topologyTopLevelFields()...)
	fields = append(fields, topologyMockWorkerEntryFields()...)
	fields = append(fields, topologyWorkInputSelectorFields()...)
	fields = append(fields, topologyScriptConfigFields()...)
	fields = append(fields, topologyRejectConfigFields()...)
	return fields
}

func topologyTopLevelFields() []FieldRecord {
	return []FieldRecord{
		{
			ID:                   "mockWorkers",
			JSONPath:             "mockWorkers",
			JSONName:             "mockWorkers",
			ValueType:            "array",
			Required:             "required",
			DefaultEmptyBehavior: "missing mockWorkers decodes as an empty slice; nil after decode is normalized to []",
			ValidationOwner:      ownerDecode,
			Notes:                "LoadMockWorkersConfig with an empty path returns NewEmptyMockWorkersConfig without reading a file",
		},
		{
			ID:                   "unmatchedDispatchPolicy",
			JSONPath:             "unmatchedDispatchPolicy",
			JSONName:             "unmatchedDispatchPolicy",
			ValueType:            "string enum",
			Required:             "optional",
			DefaultEmptyBehavior: "omitted or empty string behaves as accept for unmatched dispatches",
			ValidationOwner:      ownerValidate,
			Notes:                `accepted values are "accept" and "passthrough"`,
		},
	}
}

func topologyMockWorkerEntryFields() []FieldRecord {
	return []FieldRecord{
		{
			ID:                   "mockWorkers[].id",
			JSONPath:             "mockWorkers[].id",
			JSONName:             "id",
			ValueType:            "string",
			ParentField:          "mockWorkers[]",
			Required:             "optional",
			DefaultEmptyBehavior: "omitted id is allowed and leaves diagnostics matching unconstrained",
			ValidationOwner:      ownerDecode,
		},
		{
			ID:                   "mockWorkers[].workerName",
			JSONPath:             "mockWorkers[].workerName",
			JSONName:             "workerName",
			ValueType:            "string",
			ParentField:          "mockWorkers[]",
			Required:             "optional",
			DefaultEmptyBehavior: "omitted workerName does not constrain worker-name matching",
			ValidationOwner:      ownerDecode,
		},
		{
			ID:                   "mockWorkers[].workstationName",
			JSONPath:             "mockWorkers[].workstationName",
			JSONName:             "workstationName",
			ValueType:            "string",
			ParentField:          "mockWorkers[]",
			Required:             "optional",
			DefaultEmptyBehavior: "omitted workstationName does not constrain workstation matching",
			ValidationOwner:      ownerDecode,
		},
		{
			ID:                   "mockWorkers[].workInputs",
			JSONPath:             "mockWorkers[].workInputs",
			JSONName:             "workInputs",
			ValueType:            "array",
			ParentField:          "mockWorkers[]",
			Required:             "optional",
			DefaultEmptyBehavior: "omitted workInputs does not constrain consumed work-input selectors",
			ValidationOwner:      ownerDecode,
		},
		{
			ID:                   "mockWorkers[].runType",
			JSONPath:             "mockWorkers[].runType",
			JSONName:             "runType",
			ValueType:            "string enum",
			ParentField:          "mockWorkers[]",
			Required:             "required",
			DefaultEmptyBehavior: "missing runType fails Validate with actionable runType union message",
			ValidationOwner:      ownerValidate,
			Notes:                `accepted values are "accept", "script", and "reject"`,
		},
		{
			ID:                   "mockWorkers[].scriptConfig",
			JSONPath:             "mockWorkers[].scriptConfig",
			JSONName:             "scriptConfig",
			ValueType:            "object",
			ParentField:          "mockWorkers[]",
			Required:             "required when runType is script",
			DefaultEmptyBehavior: "omitted scriptConfig is rejected when runType is script",
			ValidationOwner:      ownerValidate,
		},
		{
			ID:                   "mockWorkers[].rejectConfig",
			JSONPath:             "mockWorkers[].rejectConfig",
			JSONName:             "rejectConfig",
			ValueType:            "object",
			ParentField:          "mockWorkers[]",
			Required:             "optional",
			DefaultEmptyBehavior: "omitted rejectConfig is allowed when runType is reject",
			ValidationOwner:      ownerValidate,
		},
	}
}

func topologyWorkInputSelectorFields() []FieldRecord {
	selectors := []struct {
		id       string
		jsonName string
	}{
		{id: "mockWorkers[].workInputs[].workId", jsonName: "workId"},
		{id: "mockWorkers[].workInputs[].workType", jsonName: "workType"},
		{id: "mockWorkers[].workInputs[].state", jsonName: "state"},
		{id: "mockWorkers[].workInputs[].inputName", jsonName: "inputName"},
		{id: "mockWorkers[].workInputs[].traceId", jsonName: "traceId"},
		{id: "mockWorkers[].workInputs[].channel", jsonName: "channel"},
		{id: "mockWorkers[].workInputs[].payloadHash", jsonName: "payloadHash"},
	}
	fields := make([]FieldRecord, 0, len(selectors))
	for _, selector := range selectors {
		fields = append(fields, FieldRecord{
			ID:                   selector.id,
			JSONPath:             selector.id,
			JSONName:             selector.jsonName,
			ValueType:            "string",
			ParentField:          "mockWorkers[].workInputs[]",
			Required:             "optional",
			DefaultEmptyBehavior: "omitted selector field does not constrain that dimension; all specified selector fields on an entry must match",
			ValidationOwner:      ownerDecode,
		})
	}
	return fields
}

func topologyScriptConfigFields() []FieldRecord {
	return []FieldRecord{
		{
			ID:                   "mockWorkers[].scriptConfig.command",
			JSONPath:             "mockWorkers[].scriptConfig.command",
			JSONName:             "command",
			ValueType:            "string",
			ParentField:          "mockWorkers[].scriptConfig",
			Required:             "required when runType is script",
			DefaultEmptyBehavior: "missing or empty command fails Validate when runType is script",
			ValidationOwner:      ownerValidate,
		},
		{
			ID:                   "mockWorkers[].scriptConfig.args",
			JSONPath:             "mockWorkers[].scriptConfig.args",
			JSONName:             "args",
			ValueType:            "array of string",
			ParentField:          "mockWorkers[].scriptConfig",
			Required:             "optional",
			DefaultEmptyBehavior: "omitted args leaves command argument list empty",
			ValidationOwner:      ownerDecode,
		},
		{
			ID:                   "mockWorkers[].scriptConfig.env",
			JSONPath:             "mockWorkers[].scriptConfig.env",
			JSONName:             "env",
			ValueType:            "object map string to string",
			ParentField:          "mockWorkers[].scriptConfig",
			Required:             "optional",
			DefaultEmptyBehavior: "omitted env leaves script environment unchanged aside from runner defaults",
			ValidationOwner:      ownerDecode,
		},
		{
			ID:                   "mockWorkers[].scriptConfig.workingDirectory",
			JSONPath:             "mockWorkers[].scriptConfig.workingDirectory",
			JSONName:             "workingDirectory",
			ValueType:            "string",
			ParentField:          "mockWorkers[].scriptConfig",
			Required:             "optional",
			DefaultEmptyBehavior: "omitted workingDirectory uses runner default working directory",
			ValidationOwner:      ownerDecode,
		},
		{
			ID:                   "mockWorkers[].scriptConfig.stdin",
			JSONPath:             "mockWorkers[].scriptConfig.stdin",
			JSONName:             "stdin",
			ValueType:            "string",
			ParentField:          "mockWorkers[].scriptConfig",
			Required:             "optional",
			DefaultEmptyBehavior: "omitted stdin leaves script stdin empty",
			ValidationOwner:      ownerDecode,
		},
		{
			ID:                   "mockWorkers[].scriptConfig.timeout",
			JSONPath:             "mockWorkers[].scriptConfig.timeout",
			JSONName:             "timeout",
			ValueType:            "string",
			ParentField:          "mockWorkers[].scriptConfig",
			Required:             "optional",
			DefaultEmptyBehavior: "omitted timeout uses runner default script timeout; duration strings such as 30s are accepted when set",
			ValidationOwner:      ownerDecode,
			Notes:                "bounds local script execution only; not a dispatch-delay authoring field",
		},
	}
}

func topologyRejectConfigFields() []FieldRecord {
	return []FieldRecord{
		{
			ID:                   "mockWorkers[].rejectConfig.stdout",
			JSONPath:             "mockWorkers[].rejectConfig.stdout",
			JSONName:             "stdout",
			ValueType:            "string",
			ParentField:          "mockWorkers[].rejectConfig",
			Required:             "optional",
			DefaultEmptyBehavior: "omitted stdout leaves rejected command stdout empty",
			ValidationOwner:      ownerDecode,
		},
		{
			ID:                   "mockWorkers[].rejectConfig.stderr",
			JSONPath:             "mockWorkers[].rejectConfig.stderr",
			JSONName:             "stderr",
			ValueType:            "string",
			ParentField:          "mockWorkers[].rejectConfig",
			Required:             "optional",
			DefaultEmptyBehavior: "omitted stderr leaves rejected command stderr empty",
			ValidationOwner:      ownerDecode,
		},
		{
			ID:                   "mockWorkers[].rejectConfig.exitCode",
			JSONPath:             "mockWorkers[].rejectConfig.exitCode",
			JSONName:             "exitCode",
			ValueType:            "integer",
			ParentField:          "mockWorkers[].rejectConfig",
			Required:             "optional",
			DefaultEmptyBehavior: "omitted exitCode leaves rejected exit code unset; when set must be between 1 and 255",
			ValidationOwner:      ownerValidate,
		},
	}
}
