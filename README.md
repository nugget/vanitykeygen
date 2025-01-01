tail -f server/matchfile.log | jq '.payload.key.authorizedString,.payload.key.fingerprint'

tail -f server/matchfile.log | jq
