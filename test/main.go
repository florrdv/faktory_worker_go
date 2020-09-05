package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	faktory "github.com/contribsys/faktory/client"
	worker "github.com/florrdv/faktory_worker_go"
)

func someFunc(ctx context.Context, args ...interface{}) error {
	help := worker.HelperFor(ctx)
	log.Printf("Working on job %s\n", help.Jid())
	//log.Printf("Context %v\n", ctx)
	//log.Printf("Args %v\n", args)
	time.Sleep(1 * time.Second)
	return nil
}

func batchFunc(ctx context.Context, args ...interface{}) error {
	help := worker.HelperFor(ctx)

	log.Printf("Working on job %s\n", help.Jid())
	if help.Bid() != "" {
		log.Printf("within %s...\n", help.Bid())
	}
	//log.Printf("Context %v\n", ctx)
	//log.Printf("Args %v\n", args)
	return nil
}

func main() {
	flags := log.Ldate | log.Ltime | log.Lmicroseconds | log.LUTC
	log.SetFlags(flags)

	mgr := worker.NewManager()
	mgr.Use(func(ctx context.Context, job *faktory.Job, next func(ctx context.Context) error) error {
		log.Printf("Starting work on job %s of type %s with custom %v\n", job.Jid, job.Type, job.Custom)
		err := next(ctx)
		log.Printf("Finished work on job %s with error %v\n", job.Jid, err)
		return err
	})

	// register job types and the function to execute them
	mgr.Register("SomeJob", someFunc)
	mgr.Register("SomeWorker", someFunc)
	mgr.Register("ImportImageJob", batchFunc)
	mgr.Register("ImportImageSuccess", batchFunc)
	//mgr.Register("AnotherJob", anotherFunc)

	// use up to N goroutines to execute jobs
	mgr.Concurrency = 20

	// pull jobs from these queues, in this order of precedence
	mgr.ProcessStrictPriorityQueues("critical", "default", "bulk")

	var quit bool
	mgr.On(worker.Shutdown, func(*worker.Manager) error {
		quit = true
		return nil
	})
	go func() {
		for {
			if quit {
				return
			}
			batch()
			produce()
			time.Sleep(1 * time.Second)
		}
	}()

	// Start processing jobs, this method does not return
	mgr.Run()
}

func batch() {
	cl, err := faktory.Open()
	if err != nil {
		panic(err)
	}

	hash, err := cl.Info()
	if err != nil {
		panic(err)
	}
	desc := hash["server"].(map[string]interface{})["description"].(string)
	if !strings.Contains(desc, "Enterprise") {
		return
	}

	// Batch example
	// We want to import all images associated with user 1234.
	// Once we've imported those two images, we want to fire
	// a success callback so we can notify user 1234.
	b := faktory.NewBatch(cl)
	b.Description = "Import images for user 1234"
	b.Success = faktory.NewJob("ImportImageSuccess", "parent", "1234")
	// Once we call Jobs(), the batch is off and running
	err = b.Jobs(func() error {
		b.Push(faktory.NewJob("ImportImageJob", "1"))

		// a child batch represents a set of jobs which can be monitored
		// separately from the parent batch's jobs. parent success won't
		// fire until child success runs without error.
		child := faktory.NewBatch(cl)
		child.ParentBid = b.Bid
		child.Description = "Child of " + b.Bid
		child.Success = faktory.NewJob("ImportImageSuccess", "child", "1234")
		err = child.Jobs(func() error {
			return child.Push(faktory.NewJob("ImportImageJob", "2"))
		})
		if err != nil {
			return err
		}
		return b.Push(faktory.NewJob("ImportImageJob", "3"))
	})
	if err != nil {
		panic(err)
	}

	st, err := cl.BatchStatus(b.Bid)
	if err != nil {
		panic(err)
	}
	fmt.Printf("%+v", st)
}

// Push something for us to work on.
func produce() {
	cl, err := faktory.Open()
	if err != nil {
		panic(err)
	}

	job := faktory.NewJob("SomeJob", 1, 2, "hello")
	job.Custom = map[string]interface{}{
		"hello": "world",
	}
	err = cl.Push(job)
	if err != nil {
		panic(err)
	}
	//fmt.Println(cl.Info())
}
