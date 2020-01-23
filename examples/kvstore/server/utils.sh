#!/usr/bin/env bash

function start3cluster() {
  if [ ! -z ${pid1+x} ]; then
    kill $pid1
    unset pid1
  fi
  if [ ! -z ${pid2+x} ]; then
    kill $pid2
    unset pid2
  fi
  if [ ! -z ${pid3+x} ]; then
    kill $pid3
    unset pid3
  fi

  go build .

  local addresses='1,tcp://localhost:8001|2,tcp://localhost:8002|3,tcp://localhost:8003'
  local common_flags=(
    -addresses=$addresses
    -enableLogging=true
    -consistency=lease
  )

  ./server -addr=:3001 -id=1 ${common_flags[@]} &
  export pid1=$!
  ./server -addr=:3002 -id=2 ${common_flags[@]} &
  export pid2=$!
  ./server -addr=:3003 -id=3 ${common_flags[@]} &
  export pid3=$!

  open "http://localhost:3001/state"
  open "http://localhost:3002/state"
  open "http://localhost:3003/state"
}

function stop3cluster() {
  kill $pid1
  kill $pid2
  kill $pid3
  unset pid1
  unset pid2
  unset pid3
}
