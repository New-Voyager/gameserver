Powerful system but I got a bit lost configuring it, any help would be appreciated.
We have configured NATS server (beta 2.2.0) to support WebSocket. configured system_account and operator resolver via URL.
In Kubernetes, We have also configured nat-account-server via directory store method (not read-only so we can POST to it), configured operatorjwtpath, systemaccountjwtpath and enabled nats notification so nat-account-server to send account updates to nat server.
3 replies

Armin Nikdel  5 hours ago
Our scenario would be like a user who login to our web-based dashboard, backend create a unique jwt for him to access specific topics by directly connecting to wss server at 9222.
Q1. how to create required parameters for JWT (sub, iss, ..)? is using nsc cli tool the only way?
Q2. Is it correct that once backend generated the JWT, post it account server so it notify nat servers so user be able to connect to websocket with his jwt? POST to nats-account-server as such:
curl -i -X POST external-nat-account-server:9090/jwt/v1/accounts/AC7PO3MREV26U3LFZFP5BN3HAI32X3PKLBRVMPAETLEHWPQEUG7EJY4H --data-binary @/Users/synadia/.nsc/nats/Test/accounts/TestAccount/TestAccount.jwt -H "Content-Type: text/text"
Q3. similar to Q1, in this example, how do we generate URL parameter? (AC7PO3MREV26U3LFZFP5BN3HAI32X3PKLBRVMPAETLEHWPQEUG7EJY4H)
Q4. when user logout, we need to invalidate his JWT so that they cannot access websocket anymore. Similar to Q2, nats-account-server and our backend will use cURL to communicate, as our backend is located in different server than nats-account-server. How to do it? What is correct cURL command to do it? (edited) 

Armin Nikdel  5 hours ago
we love how you’ve achieved it in https://www.youtube.com/watch?v=3PYFVN7dvxQ (edited) 

Alberto Ricart  3 hours ago
@Armin Nikdel additional responses:
Q1: no, the jwt library (https://github.com/nats-io/jwt) - The account, operator, and mem-resolver configuration used by the demo were created by nsc, all else was created on the fly by the web server using the JWT library.
Q2: User JWTs are not stored by account servers - only accounts and operator are. In cases where you have many users but few accounts, a mem resolver is more than sufficient.
Q3: ??? That looks like an NKEY identifying an account
Q4: User JWT doesn’t need to be invalidated like that - there are a couple of things that you can do. The cookie that carries the user JWT can be set to be HttpOnly - which means that the browser won’t show it to the user (it is not inspectable). So now it cannot be replayed. Second, the user JWTs should have short durations, short as defined whatever makes sense for an user session (probably a day). At that point you can just force authentication. Alternatively, you revoke all user JWTs generated at some point in time (last day), and then redirect the clients to re-authenticate.
nats-io/jwt
JWT tokens signed using NKeys for Ed25519 for the NATS ecosystem.
Stars
32
Language
Go
<https://github.com/nats-io/jwt|nats-io/jwt>nats-io/jwt | Aug 22nd, 2018 | Added b

https://github.com/nats-io/nats.deno

the jwt parameter In https://www.npmjs.com/package/nats is mentioned as ‘userJWT’ but in https://github.com/aricart/authdemo#natsws-auth-demo project it is set as ‘jwt’. which one is correct for nats.js to be used with nat-server 2.2.0 beta 19 as websocket? (edited) 
aricart/authdemo
Stars
2
Language
Go
<https://github.com/aricart/authdemo|aricart/authdemo>aricart/authdemo | Apr 9th | Added by GitHub
npmnpm
nats
Node.js client for NATS, a lightweight, high-performance cloud native messaging system
