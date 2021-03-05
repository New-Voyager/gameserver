# botrunner

## Generate protobuf parser

Run below command to generate go code from the protobuf files. This 
should be done once initially and whenever the proto files are updated.

```
botrunner git:(master) make compile-proto
```

## Run (bots only)

### Start the servers.

First run the other services (nats, redis, api server, game server).

```
gameserver git:(master) make build
gameserver git:(master) make run-nats
gameserver git:(master) make run-redis

apiserver git:(master) npx yarn run-pg
apiserver git:(master) npx yarn watch-debug-nats

gameserver git:(master) make run-server
```

### Botrunner (bots only)

Once the server are running, use below command to run the bot runner.

```
botrunner git:(master) make run
```

You can override the api server and nats host, I.e., for Kubernetes.
```
DEV_API_SERVER_URL=http://35.188.237.125:9501 \
    make run
```

## Botrunner (with a human)

When running a script with a human player, the botrunner will pause and wait
for the human player. You can join and drive the human component using the tester app.

```
# This botrunner will create a game and pause, giving you time to join.
# It will print the game code to join.
DEV_BOT_SCRIPT=river-action-2-bots-1-human.yaml make run

# Run the tester app from a different terminal to join the game as the human component.
GAME_CODE=XXXXXX make tester
```

Here is an example for a kubernetes run using a script containing a human.
```
export DEV_API_SERVER_URL=http://35.188.237.125:9501
export DEV_BOT_SCRIPT=river-action-2-bots-1-human.yaml

# Bots
make run

# Human
GAME_CODE=PKBC7P make tester
```

## Botrunner Server

Botrunner server allows launching multiple Botrunner instances via REST API.

Start the Botrunner server using below command.
```
make botrunner-server
```

Open a separate shell and try launching Botrunner instances using curl.
The script path is relative to the Botrunner's working directory.
```
# Launch 2 Botrunner instances with 3-second interval.
curl -i -X POST http://localhost:8081/apply -H 'content-type: application/json' -d'{"script": "botrunner_scripts/play-many-hands.yaml", "numGames": 1, "launchInterval": 3.0}'
curl -i -X POST http://localhost:8081/apply -H 'content-type: application/json' -d'{"script": "botrunner_scripts/plo-many-hands.yaml", "numGames": 1, "launchInterval": 3.0}'

# Each Botrunner logs to its own file. Tail the logs.
tail -f log/default_group/botrunner_*.log
```

Launch more Botrunners so that the total number of Botrunner instances becomes 5.
This time use 8.9 second interval between them.
```
curl -i -X POST http://localhost:8081/apply -H 'content-type: application/json' -d'{"numGames": 5, "launchInterval": 8.9}'

# Verify 3rd, 4th, and 5th logs being created.
tail -f log/default_group/botrunner_*.log
```

Reduce the number of Botrunners back down to 2. This will stop the instances 3, 4, and 5.
```
curl -i -X POST http://localhost:8081/apply -H 'content-type: application/json' -d'{"numGames": 2}'

# Monitor the log to verify the game ends after the current hand.
tail -f log/default_group/botrunner_5.log
```

Start a separate batch of Botrunners using a different script. Botrunner batches are identified by batchId string.
```
curl -i -X POST http://localhost:8081/apply -H 'content-type: application/json' -d'{"batchId": "batch_2", "script": "botrunner_scripts/some-other-script.yaml", "numGames": 2, "launchInterval": 3.0}'

curl -i -X POST http://localhost:8081/apply -H 'content-type: application/json' -d'{"batchId": "holdem", "script": "botrunner_scripts/play-many-hands.yaml", "numGames": 1, "launchInterval": 3.0}'

# Monitor the new batch of Botrunners.
tail -f log/batch_2/botrunner_*.log
```

Stop Botrunners in all batches.
```
curl -i -X POST http://localhost:8081/delete-all
```

Run a single BotRunner that joins a pre-created human game.
```
# Substitute the club code and game code.
curl -i -X POST http://localhost:8081/join-human-game'?'club-code=C-YPMXAK'&'game-code=CG-7YQTXD
```

Delete the tracker for the human-game.
```
# Substitute the game code.
curl -i -X POST http://localhost:8081/delete-human-game'?'game-code=CG-7YQTXD
```
