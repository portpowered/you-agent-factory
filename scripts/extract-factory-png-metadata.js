#!/usr/bin/env node

const fs = require("node:fs");
const path = require("node:path");

const PNG_SIGNATURE = Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]);
const PNG_TEXT_CHUNK = "tEXt";
const PNG_INTERNATIONAL_TEXT_CHUNK = "iTXt";
const PORT_OS_FACTORY_PNG_METADATA_KEYWORD = "portos.agent-factory";
const PORT_OS_FACTORY_PNG_SCHEMA_VERSION = "portos.agent-factory.png.v1";
const PORT_OS_FACTORY_PNG_ITXT_MIN_FIELDS = 5;
const PORT_OS_FACTORY_PNG_ITXT_UNCOMPRESSED_FLAG = 0;

function usage() {
  console.error(
    "Usage: node scripts/extract-factory-png-metadata.js [image-path] [output-path]\n" +
      "Defaults:\n" +
      "  image-path: ./big-logics.png\n" +
      "  output-path: stdout"
  );
}

function main(argv = process.argv.slice(2)) {
  if (argv.includes("--help") || argv.includes("-h")) {
    usage();
    return 0;
  }

  const imagePath = path.resolve(argv[0] || "big-logics.png");
  const outputPath = argv[1] ? path.resolve(argv[1]) : null;

  const pngBytes = fs.readFileSync(imagePath);
  assertPngSignature(pngBytes, imagePath);

  const metadataText = readFactoryMetadataText(pngBytes);
  const metadataEnvelope = parseFactoryMetadata(metadataText);
  const factoryPayload = stripSchemaVersion(metadataEnvelope);
  const formattedFactoryJson = `${JSON.stringify(factoryPayload, null, 2)}\n`;

  if (outputPath) {
    fs.writeFileSync(outputPath, formattedFactoryJson, "utf8");
    console.error(`Extracted factory metadata from ${imagePath} to ${outputPath}`);
    return 0;
  }

  process.stdout.write(formattedFactoryJson);
  return 0;
}

function assertPngSignature(pngBytes, imagePath) {
  if (pngBytes.length < PNG_SIGNATURE.length) {
    throw new Error(`File is too small to be a PNG: ${imagePath}`);
  }

  for (let index = 0; index < PNG_SIGNATURE.length; index += 1) {
    if (pngBytes[index] !== PNG_SIGNATURE[index]) {
      throw new Error(`File is not a PNG image: ${imagePath}`);
    }
  }
}

function readFactoryMetadataText(pngBytes) {
  let offset = PNG_SIGNATURE.length;

  while (offset < pngBytes.length) {
    const chunk = readChunkAtOffset(pngBytes, offset);
    offset = chunk.nextOffset;

    if (chunk.type !== PNG_TEXT_CHUNK && chunk.type !== PNG_INTERNATIONAL_TEXT_CHUNK) {
      continue;
    }

    const textChunk = readTextChunk(chunk.type, chunk.data);
    if (textChunk.keyword !== PORT_OS_FACTORY_PNG_METADATA_KEYWORD) {
      continue;
    }

    return textChunk.text;
  }

  throw new Error("PNG does not contain Infinite You factory metadata.");
}

function readChunkAtOffset(pngBytes, offset) {
  if (offset + 8 > pngBytes.length) {
    throw new Error("PNG chunk header is truncated.");
  }

  const length = pngBytes.readUInt32BE(offset);
  const dataOffset = offset + 8;
  const dataEnd = dataOffset + length;
  const chunkEnd = dataEnd + 4;

  if (chunkEnd > pngBytes.length) {
    throw new Error("PNG chunk payload is truncated.");
  }

  return {
    data: pngBytes.subarray(dataOffset, dataEnd),
    nextOffset: chunkEnd,
    type: pngBytes.subarray(offset + 4, offset + 8).toString("ascii"),
  };
}

function readTextChunk(type, data) {
  if (type === PNG_TEXT_CHUNK) {
    return readTextKeywordAndValue(data);
  }

  return readInternationalTextKeywordAndValue(data);
}

function readTextKeywordAndValue(data) {
  const keywordEnd = data.indexOf(0);
  if (keywordEnd < 0) {
    throw new Error("PNG tEXt chunk keyword is invalid.");
  }

  return {
    keyword: data.subarray(0, keywordEnd).toString("ascii"),
    text: data.subarray(keywordEnd + 1).toString("utf8"),
  };
}

function readInternationalTextKeywordAndValue(data) {
  const keywordEnd = data.indexOf(0);
  if (keywordEnd < 0) {
    throw new Error("PNG iTXt chunk keyword is invalid.");
  }

  if (data.length < keywordEnd + PORT_OS_FACTORY_PNG_ITXT_MIN_FIELDS + 1) {
    throw new Error("PNG iTXt chunk is truncated.");
  }

  const compressionFlagIndex = keywordEnd + 1;
  const compressionFlag = data[compressionFlagIndex];
  if (compressionFlag !== PORT_OS_FACTORY_PNG_ITXT_UNCOMPRESSED_FLAG) {
    throw new Error("Compressed PNG iTXt metadata is not supported.");
  }

  const languageTagEnd = data.indexOf(0, keywordEnd + 3);
  if (languageTagEnd < 0) {
    throw new Error("PNG iTXt chunk language tag is invalid.");
  }

  const translatedKeywordEnd = data.indexOf(0, languageTagEnd + 1);
  if (translatedKeywordEnd < 0) {
    throw new Error("PNG iTXt chunk translated keyword is invalid.");
  }

  return {
    keyword: data.subarray(0, keywordEnd).toString("ascii"),
    text: data.subarray(translatedKeywordEnd + 1).toString("utf8"),
  };
}

function parseFactoryMetadata(metadataText) {
  let parsedMetadata;
  try {
    parsedMetadata = JSON.parse(metadataText);
  } catch (error) {
    throw new Error(`Factory metadata is not valid JSON: ${error.message}`);
  }

  if (!isRecord(parsedMetadata)) {
    throw new Error("Factory metadata must be a JSON object.");
  }

  if (!isNonEmptyString(parsedMetadata.schemaVersion)) {
    throw new Error("Factory metadata is missing schemaVersion.");
  }

  if (parsedMetadata.schemaVersion !== PORT_OS_FACTORY_PNG_SCHEMA_VERSION) {
    throw new Error(
      `Unsupported factory metadata schemaVersion: ${parsedMetadata.schemaVersion}`
    );
  }

  if (!isNonEmptyString(parsedMetadata.name)) {
    throw new Error("Factory metadata is missing the factory name.");
  }

  return parsedMetadata;
}

function stripSchemaVersion(metadataEnvelope) {
  const { schemaVersion: _schemaVersion, ...factoryPayload } = metadataEnvelope;
  return factoryPayload;
}

function isNonEmptyString(value) {
  return typeof value === "string" && value.trim().length > 0;
}

function isRecord(value) {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

try {
  process.exitCode = main();
} catch (error) {
  console.error(error.message);
  process.exitCode = 1;
}
