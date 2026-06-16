package workflowruntime

type childRecordSink struct {
	records *recordCollector
}

func childRecordSinkFromCollector(records *recordCollector) ChildRecordSink {
	if records == nil {
		return childRecordSink{records: newRecordCollector()}
	}
	return childRecordSink{records: records}
}

func (s childRecordSink) Append(record RuntimeRecord) {
	s.records.append(record)
}

func (s childRecordSink) AppendChildDispatch(base ChildDispatchRecord, status string) {
	record := base
	record.Status = status
	s.Append(RuntimeRecord{
		Kind:          RecordKindChildDispatch,
		ChildDispatch: &record,
	})
}

func (s childRecordSink) NextChildDispatchIdentity() (string, int) {
	return s.records.nextChildDispatchIdentity()
}

func (s childRecordSink) NextChildArtifactID() string {
	return s.records.nextChildArtifactID()
}
