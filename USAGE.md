Usage:
  -access string
    	Access config file (default "./access.yaml")
  -debug
    	Debug logs
  -idle-timeout duration
    	Idle session timeout (default 3m0s)
  -listen string
    	Listen address/port (default ":2222")
  -overall-timeout duration
    	Overall session timeout
  -private value
    	Host private key file, repeatable
  -socket-network string
    	Reverse tunnel socket network (default "tcp")

Signals:
  SIGHUP - Reload access config
