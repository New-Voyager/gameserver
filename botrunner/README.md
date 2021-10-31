# botrunner

## Generate protobuf parser

Run below command to generate go code from the protobuf files. This 
should be done once initially and whenever the proto files are updated.

```
game-server git:(master) make compile-proto
```

## Run (bots only)

### Start the servers.

First run the other services (nats, redis, api server, game server, timer, scheduler).

```
apiserver git:(master) make run-all
apiserver git:(master) make debug

timer git:(master) make run
scheduler git:(master) make run
server git:(master) make run-server
```

### Botrunner (bots only)

Once the server are running, use below command to run the bot runner.

```
botrunner git:(master) BOTRUNNER_SCRIPT=./botrunner_scripts/system_test/parallel/basic/river-action-3-bots.yaml make run
```

You can override the api server and nats host, I.e., for Kubernetes.
```
DEV_API_SERVER_URL=https://api.pokerapp.club \
    DEV_NATS_URL=nats://nats-0.pokerapp.club:4222,nats://nats-1.pokerapp.club:4222,nats://nats-2.pokerapp.club:4222 \
    BOTRUNNER_SCRIPT=botrunner_scripts/load_test/holdem-load-test.yaml \
    make run
```

## Botrunner (with a human)

When running a script with a human player, the botrunner will pause and wait
for the human player. You can join and drive the human component using the tester app.

```
# This botrunner will create a game and pause, giving you time to join.
# It will print the game code to join.
BOTRUNNER_SCRIPT=botrunner_scripts/human_game/river-action-2-bots-1-human.yaml make run

# Run the tester app from a different terminal to join the game as the human component.
GAME_CODE=XXXXXX make tester
```

Here is an example for a kubernetes run using a script containing a human.
```
export DEV_API_SERVER_URL=http://35.188.237.125:9501
export DEV_API_SERVER_INTERNAL_URL=http://35.188.237.125:9502
export DEV_NATS_URL=nats://35.188.237.125:4222
export BOTRUNNER_SCRIPT=botrunner_scripts/human_game/river-action-2-bots-1-human.yaml

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
# Launch 100 Botrunner instances with 3-second interval.
curl -i -X POST http://localhost:8081/apply -H 'content-type: application/json' -d'{"batchId": "holdem", "script": "botrunner_scripts/load_test/holdem-load-test.yaml", "numGames": 100, "launchInterval": 3.0}'
curl -i -X POST http://localhost:8081/apply -H 'content-type: application/json' -d'{"batchId": "plo", "script": "botrunner_scripts/load_test/plo-load-test.yaml", "numGames": 100, "launchInterval": 3.0}'

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

curl -i -X POST http://localhost:8081/apply -H 'content-type: application/json' -d'{"batchId": "holdem", "script": "botrunner_scripts/many_hands/play-many-hands.yaml", "numGames": 1, "launchInterval": 3.0}'

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

## System Test

Run system test.
```
make system-test-build && make system-test
```

Get the code coverage report after the system test is run.
```
make system-test-coverage
```

To run a bot game.
```
POKER_LOCAL_IP=192.168.0.106 BOTRUNNER_SCRIPT=botrunner_scripts/system_test/river-action-3-bots.yaml make botrunner
```

