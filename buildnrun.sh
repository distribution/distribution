sudo docker build -t distribution/distribution:edge .
sudo docker run -d -p 10500:5000 -v $PWD/FS/PATH:/var/lib/registry --restart always --name registry distribution/distribution:edge