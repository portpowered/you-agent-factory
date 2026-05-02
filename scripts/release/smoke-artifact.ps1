param(
    [Parameter(Mandatory = $true)]
    [string]$BinaryPath,
    [Parameter(Mandatory = $true)]
    [string]$FixturePath,
    [string]$Timeout = "60s"
)

go run ./cmd/releasesmoke -binary $BinaryPath -fixture $FixturePath -timeout $Timeout
