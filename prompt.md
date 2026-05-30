[PDF 1] The document for this hackathon project is slightly outdated, but otherwise I have some information on how we adjusted this project.

The idea of this application is that we attack an HTTP/ws/tcp/whatever endpoint and try to find an exploit.

There's a valid Kubernetes cluster running on DigitalOcean, and you can push images to my DigitalOcean Image Registry. You are free to manipulate either as you need to. This project is will be written in Go Lang.

We want to use PI (https://pi.dev/) as the agent used penetrator, with Gemma 4 as the model being used. We want to use OpenRouter for the model hosting. There's a valid API key in the hidden folder.

We have to use Nimble.ai for something. We do have an api key for it also in the hidden folder. This can be used to navigate the site it's attacking, or to research CVE feeds. https://docs.nimbleway.com/llms.txt

We want to keep running these agents until we get an Outcome, and then write a short summary on how we came to that conclusion.

My assumption is that we need a small control plane for openzerg to start up the agents and pass in the endpoint and receive the outcome. Something I pass a ENV argument for the endpoint.

I want start by writing a PRD.json to pipe into a ralph loop. The Ralph loop will run on OpenCode and use the model Opus 4.7. 