package cursorstorage

func loadSessionFromStoreDBWithStats(dbPath string) (map[string]*RawBubble, []*RawComposer, map[string][]*MessageContext, SessionParseStats, error) {
	db, err := OpenDatabase(dbPath)
	if err != nil {
		return nil, nil, nil, SessionParseStats{}, err
	}
	defer func() { _ = db.Close() }()

	blobs, err := QueryBlobsTable(db)
	if err != nil {
		return nil, nil, nil, SessionParseStats{}, err
	}
	meta, err := QueryMetaTable(db)
	if err != nil {
		return nil, nil, nil, SessionParseStats{}, err
	}

	bubbles, composers, contexts, err := LoadSessionFromStoreDB(dbPath)
	if err != nil {
		return nil, nil, nil, SessionParseStats{}, err
	}

	stats := SessionParseStats{
		BlobCount:            len(blobs),
		ReadableBlobCount:    len(bubbles),
		UnavailableBlobCount: max(0, len(blobs)-len(bubbles)),
		MetaCount:            len(meta),
	}
	return bubbles, composers, contexts, stats, nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
