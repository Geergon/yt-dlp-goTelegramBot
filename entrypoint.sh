#!/bin/sh

# Запуск cron у фоні
crond -b -l 8

# Запуск основної програми
exec ./main
