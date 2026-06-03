package cursorstorage

func loadSessionFromStoreDBWithStats(dbPath string) (map[string]*RawBubble, []*RawComposer, map[string][]*MessageContext, SessionParseStats, SessionTokenUsage, error) {
	db, err := OpenDatabase(dbPath)
	if err != nil {
		return nil, nil, nil, SessionParseStats{}, SessionTokenUsage{}, err
	}
	defer func() { _ = db.Close() }()

	blobs, err := QueryBlobsTable(db)
	if err != nil {
		return nil, nil, nil, SessionParseStats{}, SessionTokenUsage{}, err
	}
	meta, err := QueryMetaTable(db)
	if err != nil {
		return nil, nil, nil, SessionParseStats{}, SessionTokenUsage{}, err
	}

	bubbles, composers, contexts, tokenUsage, err := LoadSessionFromStoreDB(dbPath)
	if err != nil {
		return nil, nil, nil, SessionParseStats{}, SessionTokenUsage{}, err
	}

	stats := SessionParseStats{
		BlobCount:            len(blobs),
		ReadableBlobCount:    len(bubbles),
		UnavailableBlobCount: max(0, len(blobs)-len(bubbles)),
		MetaCount:            len(meta),
	}
	return bubbles, composers, contexts, stats, tokenUsage, nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
