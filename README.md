api-example-bones
=================

An example API app in Go that checks a user's WNT16 and FAM3C genes for unbreakable bones. Try it: http://bones.herokuapp.com.

Clone the repository, and look at the ```.env``` file to see what environment variables must be set.

Your env's ```REDIRECT_URI``` must match the one on your developer dashboard at https://api.23andme.com/dashboard/. Locally you'll probably want ```http://localhost:PORT/receive_code/```.

Heroku
===
I host the app on Heroku. 
- Setup Go on Heroku: http://mmcgrana.github.com/2012/09/getting-started-with-go-on-heroku.html.
- Set environment variables on Heroku: https://devcenter.heroku.com/articles/config-vars. 

I change ```CLIENT_ID``` and ```CLIENT_SECRET``` to match my API credentials, and delete the ```PORT``` variable because Heroku sets it automatically.

```
heroku config:set CLIENT_ID=xxx
heroku config:set CLIENT_SECRET=xxx
heroku config:unset PORT
```

Local
===

Just set your environment variables, build the program, and run it:

```go
go build
./api-example-bones
```
